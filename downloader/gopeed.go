// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

// GopeedDownloader 通过 Gopeed REST API 下发下载任务（磁力/直链）并轮询完成，
// 下载产物由 Gopeed 落到 DownloadDir，下载器再把产物移动到 obj.SavePath。
type GopeedDownloader struct {
	rpcURL       string
	downloadDir  string
	pollInterval time.Duration
	timeout      time.Duration
	httpClient   *http.Client
	status       *statusWriter // 下载状态持久化（文件 JSON，可空=不落盘）
	tempSuffix   string        // 落盘中间文件名后缀（默认 .download）
}

// defaultTempSuffix 默认中间文件后缀（标识下载中，成功后改名）。
const defaultTempSuffix = ".download"

// Ensure GopeedDownloader implements core.Downloader
var _ core.Downloader = &GopeedDownloader{}

// gopeedCreateTaskRequest 是 Gopeed POST /api/v1/tasks 的请求体。
type gopeedCreateTaskRequest struct {
	ReqID string                `json:"reqId"`
	Req   gopeedCreateTaskReq   `json:"req"`
	Opts  *gopeedCreateTaskOpts `json:"opts"`
}

type gopeedCreateTaskReq struct {
	URL      string         `json:"url"`
	Protocol string         `json:"protocol"`
	Extra    map[string]any `json:"extra"`
}

// gopeedCreateTaskOpts 是任务选项（下载目录/文件名等）。
type gopeedCreateTaskOpts struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// gopeedResponse 是 Gopeed API 的统一响应包装 {code, message, data}。
type gopeedResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// gopeedTask 是 GET /api/v1/tasks/{id} 返回的任务对象。
type gopeedTask struct {
	ID       string         `json:"id"`
	Status   string         `json:"status"` // running / done / error / ...
	Meta     gopeedMeta     `json:"meta"`
	Progress gopeedProgress `json:"progress"`
}

// gopeedProgress 是任务进度（downloaded/total/speed）。
type gopeedProgress struct {
	Downloaded int64 `json:"downloaded"`
	Total      int64 `json:"total"`
	Speed      int64 `json:"speed"`
}

type gopeedMeta struct {
	Res gopeedRes `json:"res"`
}

type gopeedRes struct {
	Files []gopeedFile `json:"files"`
	Pro   int          `json:"pro"` // 进度百分比 0-100
}

type gopeedFile struct {
	Name string `json:"name"`
}

// NewGopeedDownloader 创建 Gopeed 后端下载器。
func NewGopeedDownloader(cfg config.Downloader) *GopeedDownloader {
	rpcURL := cfg.Gopeed.RPCURL
	if rpcURL == "" {
		rpcURL = config.DefaultGopeedRPCURL
	}
	pollInterval := time.Duration(cfg.Gopeed.PollIntervalSecs) * time.Second
	if pollInterval <= 0 {
		pollInterval = time.Duration(config.DefaultPollIntervalSecs) * time.Second
	}
	timeout := time.Duration(cfg.Gopeed.TimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(config.DefaultTimeoutSecs) * time.Second
	}

	return &GopeedDownloader{
		rpcURL:       strings.TrimRight(rpcURL, "/"),
		downloadDir:  cfg.Gopeed.DownloadDir,
		pollInterval: pollInterval,
		timeout:      timeout,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		status:       newStatusWriter(cfg.Gopeed.StatusFile),
		tempSuffix:   cfg.Gopeed.TempSuffix,
	}
}

func (d *GopeedDownloader) Name() string {
	return "gopeed"
}

// Download 通过 Gopeed 下载 obj.URL 到 obj.SavePath，完成后返回 nil。
//
// 三种路径：
//  1. PikPak 分享（keepshare.org/<id>/magnet:... 或 mypikpak.com/s/...）：
//     先解析出免登录直链（POST /api/v1/resolve 触发 pikpak 扩展），再交给 Gopeed http 下载。
//  2. 磁力（magnet:/bt:）：POST /api/v1/tasks 创建磁力任务，轮询直到完成。
//  3. 普通 http(s)：直接创建任务下载。
func (d *GopeedDownloader) Download(obj *model.DownloadObject, headers map[string]string) error {
	// 优先从 obj.Extra.files 读下载源：任务（如 njavtv）可能把 keepshare/magnet 链接
	// 放在 files[0].url（磁力全长优先），obj.URL 保持详情页身份键不变。
	if u := d.firstPikPakFromFiles(obj); u != "" {
		target := model.DownloadObject{TaskID: obj.TaskID, URL: u, SavePath: obj.SavePath}
		return d.downloadViaPikPak(&target)
	}
	if d.isPikPakURL(obj.URL) {
		return d.downloadViaPikPak(obj)
	}

	taskID, err := d.createTask(obj)
	if err != nil {
		d.recordTaskError(taskID, obj, "error", err.Error())
		return fmt.Errorf("gopeed create task: %w", err)
	}

	slog.Info("Gopeed task created", "task_id", taskID, logutil.LogKeyURL, obj.URL)
	// 统一走 waitAndMove：轮询进度持久化 + done/error/timeout 终态记录。
	return d.waitAndMove(obj, taskID)
}

// createTask 调用 POST {rpcURL}/api/v1/tasks 下发任务，返回 task id。
func (d *GopeedDownloader) createTask(obj *model.DownloadObject) (string, error) {
	req := gopeedCreateTaskRequest{
		ReqID: fmt.Sprintf("req-%d", time.Now().UnixNano()),
		Req: gopeedCreateTaskReq{
			URL:      obj.URL,
			Protocol: resolveProtocol(obj.URL),
			Extra:    map[string]any{},
		},
		// 下载目录：优先 obj.SavePath 所在目录（受控落盘），回退配置 DownloadDir。
		// 文件名：任务指定 SavePath 文件名 + TempSuffix（中间后缀标识下载中），
		// 下载成功后 moveResult 改名为最终名（避免 Gopeed 推断 'download'）。
		Opts: &gopeedCreateTaskOpts{Path: d.resolveDownloadDir(obj), Name: d.tempName(obj)},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal create task body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, d.rpcURL+"/api/v1/tasks", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	var resp gopeedResponse
	if err := d.doRequest(httpReq, &resp); err != nil {
		return "", err
	}
	var data string
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("decode create task response data: %w", err)
	}
	if data == "" {
		return "", fmt.Errorf("gopeed create task returned empty task id")
	}
	return data, nil
}

// getTask 查询 {rpcURL}/api/v1/tasks/{id} 并解码任务对象。
func (d *GopeedDownloader) getTask(taskID string) (*gopeedTask, error) {
	u := d.rpcURL + "/api/v1/tasks/" + taskID
	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var resp gopeedResponse
	if err := d.doRequest(httpReq, &resp); err != nil {
		return nil, err
	}
	var task gopeedTask
	if err := json.Unmarshal(resp.Data, &task); err != nil {
		return nil, fmt.Errorf("decode task response data: %w", err)
	}
	return &task, nil
}

// doRequest 执行 HTTP 请求并解析 Gopeed 统一响应包装，非零 code 视为失败。
func (d *GopeedDownloader) doRequest(req *http.Request, out *gopeedResponse) error {
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gopeed API returned HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if out.Code != 0 {
		return fmt.Errorf("gopeed API error code=%d message=%q", out.Code, out.Message)
	}
	return nil
}

// moveResult 把 Gopeed 下载产物移动到 obj.SavePath（任务指定的最终文件名/目录）。
// 产物定位：优先 Gopeed 任务返回的 files[0].name；
// 找不到（文件名冲突/被 Gopeed 加后缀）时扫描下载目录找最新修改文件。
func (d *GopeedDownloader) moveResult(obj *model.DownloadObject, taskID string) error {
	task, err := d.getTask(taskID)
	if err != nil {
		return fmt.Errorf("gopeed get task result: %w", err)
	}

	// 优先：Gopeed 落盘中间文件名（Path/Name = SavePath 文件名 + TempSuffix）
	srcPath := d.tempResultPath(obj)
	if srcPath == "" || !fileExists(srcPath) {
		// 回退：files[0].name 或目录最新文件（Gopeed 未按 Name 落盘时）
		srcPath = d.resultPath(obj, task.Meta.Res.Files)
		if srcPath == "" {
			return fmt.Errorf("gopeed: unable to resolve downloaded file path for url %s", obj.URL)
		}
		if !fileExists(srcPath) {
			if latest := d.latestFileInDir(obj); latest != "" {
				srcPath = latest
			}
		}
	}
	if obj.SavePath == "" {
		return fmt.Errorf("gopeed: obj.SavePath is empty")
	}
	// 产物已就位（Gopeed 直接落盘到 SavePath 同名）→ 无需移动
	if filepath.Clean(srcPath) == filepath.Clean(obj.SavePath) {
		return nil
	}
	dir := filepath.Dir(obj.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("gopeed create save dir: %w", err)
	}

	if err := os.Rename(srcPath, obj.SavePath); err != nil {
		// 跨设备/文件系统 rename 会失败，回退到复制再删除。
		if copyErr := copyFile(srcPath, obj.SavePath); copyErr != nil {
			return fmt.Errorf("gopeed move %s -> %s: rename %v, copy %w", srcPath, obj.SavePath, err, copyErr)
		}
		_ = os.Remove(srcPath)
	}
	return nil
}

// latestFileInDir 扫描下载目录，返回最近修改的常规文件（Gopeed 落盘产物）。
func (d *GopeedDownloader) latestFileInDir(obj *model.DownloadObject) string {
	dir := d.resolveDownloadDir(obj)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var best string
	var bestTime time.Time
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(bestTime) {
			best = filepath.Join(dir, e.Name())
			bestTime = info.ModTime()
		}
	}
	return best
}

// resultPath 由任务的产物文件列表解析来源路径；找不到时回退到 URL basename。
// 下载目录与 Gopeed 落盘一致：优先 obj.SavePath 所在目录（resolveDownloadDir），
// 而非全局 DownloadDir —— 保证 moveResult 同区找到产物。
func (d *GopeedDownloader) resultPath(obj *model.DownloadObject, files []gopeedFile) string {
	dlDir := d.resolveDownloadDir(obj)
	for _, f := range files {
		if f.Name == "" {
			continue
		}
		return filepath.Join(dlDir, f.Name)
	}
	base := filepath.Base(obj.URL)
	return filepath.Join(dlDir, base)
}

// isPikPakURL 判断 URL 是否 PikPak 分享类：
//   - mypikpak.com/s/<share_id>（直接分享）
//   - keepshare.org/<id>/magnet:...（keepshare 磁力镜像，302 → PikPak 分享页）
func (d *GopeedDownloader) isPikPakURL(url string) bool {
	u := strings.ToLower(url)
	// 识别 PikPak 分享（mypikpak.com/s/ 或 mypikpak.net/s/）与 keepshare 镜像
	// （keepshare.org/ 域名，或任意主机下含 /keepshare 路径 —— 后者便于测试用 mock 主机）。
	return strings.Contains(u, "mypikpak.com/s/") ||
		strings.Contains(u, "mypikpak.net/s/") ||
		strings.Contains(u, "keepshare.org/") ||
		strings.Contains(u, "/keepshare")
}

// downloadViaPikPak 处理 PikPak 分享下载：
//  1. keepshare URL 先 302 跟随取真实 mypikpak.com/s/<share_id>；
//  2. POST {rpcURL}/api/v1/resolve（Gopeed 触发 pikpak 扩展，免登录解析）→ 文件列表（含直链）；
//  3. 按 obj.URL 里的磁力 dn（或视频名）匹配目标文件，取直链 + 下载 header；
//  4. 把直链交给 Gopeed http 任务下载到 obj.SavePath（复用 createTask+轮询+moveResult）。
func (d *GopeedDownloader) downloadViaPikPak(obj *model.DownloadObject) error {
	shareURL, err := d.resolvePikPakShareURL(obj.URL)
	if err != nil {
		d.recordTaskError("", obj, "error", err.Error())
		return err
	}

	files, err := d.pikpakResolve(shareURL)
	if err != nil {
		msg := fmt.Sprintf("gopeed pikpak resolve %s: %v", shareURL, err)
		d.recordTaskError("", obj, "error", msg)
		return fmt.Errorf("gopeed pikpak resolve %s: %w", shareURL, err)
	}
	if len(files) == 0 {
		msg := fmt.Sprintf("gopeed pikpak resolve %s: no files", shareURL)
		d.recordTaskError("", obj, "error", msg)
		return fmt.Errorf("%s", msg)
	}

	target := d.pickPikPakTarget(files, obj.URL)
	if target == nil {
		msg := fmt.Sprintf("gopeed pikpak: no matching file for url %s (resolved %d files)", obj.URL, len(files))
		d.recordTaskError("", obj, "error", msg)
		return fmt.Errorf("%s", msg)
	}
	if target.DownloadURL == "" {
		msg := fmt.Sprintf("gopeed pikpak: file %q has no download url", target.Name)
		d.recordTaskError("", obj, "error", msg)
		return fmt.Errorf("%s", msg)
	}

	slog.Info("PikPak resolved target", "name", target.Name, "size", target.Size, "url", target.DownloadURL[:min(80, len(target.DownloadURL))])

	// 用直链走 Gopeed http 下载；headers（Referer/UA）通过 obj 的下载头透传。
	targetObj := model.DownloadObject{TaskID: obj.TaskID, URL: target.DownloadURL, SavePath: obj.SavePath}
	if err := d.downloadHTTP(&targetObj, target.Headers); err != nil {
		return fmt.Errorf("gopeed pikpak download %s: %w", target.Name, err)
	}
	return nil
}

// pikpakResolve 调 Gopeed /api/v1/resolve 触发扩展解析，返回文件直链列表。
func (d *GopeedDownloader) pikpakResolve(shareURL string) ([]pikpakFile, error) {
	body := fmt.Sprintf(`{"req":{"url":%q,"extra":{}},"opts":{}}`, shareURL)
	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, d.rpcURL+"/api/v1/resolve", bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	var resp gopeedResponse
	if err := d.doRequest(httpReq, &resp); err != nil {
		return nil, err
	}
	var rr struct {
		Res struct {
			Files []struct {
				Name string `json:"name"`
				Size int64  `json:"size"`
				Path string `json:"path"`
				Req  struct {
					URL   string         `json:"url"`
					Extra map[string]any `json:"extra"`
				} `json:"req"`
			} `json:"files"`
		} `json:"res"`
	}
	if err := json.Unmarshal(resp.Data, &rr); err != nil {
		return nil, fmt.Errorf("decode resolve response: %w", err)
	}
	var out []pikpakFile
	for _, f := range rr.Res.Files {
		hdrs := map[string]string{}
		if h, ok := f.Req.Extra["header"].(map[string]any); ok {
			for k, v := range h {
				hdrs[k] = fmt.Sprint(v)
			}
		}
		out = append(out, pikpakFile{Name: f.Name, Size: f.Size, Path: f.Path, DownloadURL: f.Req.URL, Headers: hdrs})
	}
	return out, nil
}

// pikpakFile 是 PikPak 解析出的一个文件及直链。
type pikpakFile struct {
	Name        string
	Size        int64
	Path        string
	DownloadURL string
	Headers     map[string]string
}

// resolvePikPakShareURL 把 keepshare 磁力镜像 URL 跟随重定向解析成真实 PikPak 分享 URL；
// 已是 mypikpak.com/s/ 的直接返回。
func (d *GopeedDownloader) resolvePikPakShareURL(url string) (string, error) {
	if strings.Contains(strings.ToLower(url), "mypikpak.com/s/") || strings.Contains(strings.ToLower(url), "mypikpak.net/s/") {
		return url, nil
	}
	// keepshare.org/<id>/magnet:... → 302 到 mypikpak.com/s/<share_id>（GET 跟随）。
	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := d.httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusTemporaryRedirect {
		final := resp.Request.URL.String()
		if final != "" && final != url {
			return final, nil
		}
	}
	return "", fmt.Errorf("keepshare url %s did not redirect to PikPak share", url)
}

// pickPikPakTarget 从解析出的文件中挑目标：优先名字与 obj.URL 中磁力 dn 匹配的视频；
// 其次选最大的 mp4（全长视频）。
func (d *GopeedDownloader) pickPikPakTarget(files []pikpakFile, rawURL string) *pikpakFile {
	// 磁力 dn（如 SAMPLE-123-uncensored-HD）→ 期望文件名；兼容 keepshare 直链或 ?dn= query 形态
	dn := ""
	if _, after, ok := strings.Cut(rawURL, "&dn="); ok {
		dn = after
	} else if _, after, ok := strings.Cut(rawURL, "?dn="); ok {
		dn = after
	}
	if dn != "" {
		want := strings.ToLower(dn)
		for i := range files {
			if strings.Contains(strings.ToLower(files[i].Name), want) {
				return &files[i]
			}
		}
	}
	// 兜底：取最大的 .mp4（全长视频通常最大）
	var best *pikpakFile
	for i := range files {
		if strings.HasSuffix(strings.ToLower(files[i].Name), ".mp4") && (best == nil || files[i].Size > best.Size) {
			best = &files[i]
		}
	}
	return best
}

// downloadHTTP 用 Gopeed 下载 http(s) 直链（普通任务路径，含 moveResult）。
func (d *GopeedDownloader) downloadHTTP(obj *model.DownloadObject, extraHeaders map[string]string) error {
	taskID, err := d.createTask(obj)
	if err != nil {
		return fmt.Errorf("gopeed create http task: %w", err)
	}
	return d.waitAndMove(obj, taskID)
}

// waitAndMove 轮询任务直到 done 并把产物移到 obj.SavePath。
func (d *GopeedDownloader) waitAndMove(obj *model.DownloadObject, taskID string) error {
	deadline := time.Now().Add(d.timeout)
	lastStatus := ""
	var lastProg gopeedProgress
	for {
		status, prog, err := d.pollTaskState(taskID)
		if err != nil {
			d.recordTaskError(taskID, obj, "error", err.Error())
			return fmt.Errorf("gopeed poll task %s: %w", taskID, err)
		}
		lastStatus = status
		if prog.Downloaded > 0 || prog.Total > 0 {
			lastProg = prog // 保留最近一次有进度的轮询
		}
		// 轮询进度持久化（避免静默）
		d.recordTaskState(taskID, obj.URL, obj.SavePath, status, lastProg.Downloaded, lastProg.Total, lastProg.Speed, "")
		switch status {
		case "done":
			slog.Info("Gopeed task done", "task_id", taskID, logutil.LogKeyURL, obj.URL)
			d.recordTaskState(taskID, obj.URL, obj.SavePath, "done", lastProg.Downloaded, lastProg.Total, 0, "")
			return d.moveResult(obj, taskID)
		case "error":
			msg := fmt.Sprintf("gopeed task %s failed: status=error", taskID)
			d.recordTaskError(taskID, obj, "error", msg)
			return fmt.Errorf("%s", msg)
		}
		if time.Now().After(deadline) {
			msg := fmt.Sprintf("gopeed task %s timed out after %s (last status %q)", taskID, d.timeout, lastStatus)
			d.recordTaskError(taskID, obj, "timeout", msg)
			return fmt.Errorf("%s", msg)
		}
		select {
		case <-time.After(d.pollInterval):
		case <-context.Background().Done():
			msg := fmt.Sprintf("gopeed task %s cancelled", taskID)
			d.recordTaskError(taskID, obj, "cancelled", msg)
			return fmt.Errorf("%s", msg)
		}
	}
}

// pollTaskState 查询任务状态与进度（downloaded/total/speed）。
func (d *GopeedDownloader) pollTaskState(taskID string) (string, gopeedProgress, error) {
	task, err := d.getTask(taskID)
	if err != nil {
		return "", gopeedProgress{}, err
	}
	return task.Status, task.Progress, nil
}

// resolveProtocol 按 URL 前缀选择 Gopeed 协议类型，无法识别时回退 "default"。
func resolveProtocol(url string) string {
	switch {
	case strings.HasPrefix(url, "magnet:"), strings.HasPrefix(url, "magnetic:"),
		strings.HasPrefix(url, "bt:"), strings.HasPrefix(url, "ed2k:"):
		return "magnet"
	case strings.HasPrefix(url, "http://"), strings.HasPrefix(url, "https://"):
		return "http"
	default:
		return "default"
	}
}

// copyFile 复制 src 到 dst（rename 跨设备失败时的回退）。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// firstPikPakFromFiles 从 obj.Extra.files 中找第一个 keepshare/magnet 下载源。
// 任务（njavtv）把磁力全长来源放 files[0].url + 标记，obj.URL 保持详情页身份。
func (d *GopeedDownloader) firstPikPakFromFiles(obj *model.DownloadObject) string {
	if obj == nil {
		return ""
	}
	obj.RLock()
	defer obj.RUnlock()
	raw, ok := obj.Extra["files"].([]map[string]string)
	if !ok {
		if fa, ok2 := obj.Extra["files"].([]any); ok2 {
			for _, it := range fa {
				if fm, ok3 := it.(map[string]any); ok3 {
					if u, _ := fm["url"].(string); u != "" && d.isPikPakURL(u) {
						return u
					}
				}
			}
			return ""
		}
		return ""
	}
	for _, f := range raw {
		if u := strings.TrimSpace(f["url"]); u != "" && d.isPikPakURL(u) {
			return u
		}
	}
	return ""
}

// resolveDownloadDir 决定 Gopeed 任务的下载目录：
//   - obj.SavePath 非空 → 其所在目录（受控，便于 moveResult 同区移动）
//   - 否则 → 配置 DownloadDir（空则由 Gopeed 默认）
func (d *GopeedDownloader) resolveDownloadDir(obj *model.DownloadObject) string {
	if obj != nil && obj.SavePath != "" {
		dir := filepath.Dir(obj.SavePath)
		if dir != "" && dir != "." {
			return dir
		}
	}
	return d.downloadDir
}

// tempName 计算 Gopeed 落盘中间文件名：SavePath 文件名 + TempSuffix（默认 .download）。
func (d *GopeedDownloader) tempName(obj *model.DownloadObject) string {
	if obj == nil || obj.SavePath == "" {
		return ""
	}
	base := filepath.Base(obj.SavePath)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return ""
	}
	suffix := d.tempSuffix
	if suffix == "" {
		suffix = defaultTempSuffix
	}
	return base + suffix
}

// tempResultPath 由中间文件名定位 Gopeed 落盘产物路径。
func (d *GopeedDownloader) tempResultPath(obj *model.DownloadObject) string {
	dir := d.resolveDownloadDir(obj)
	name := d.tempName(obj)
	if name == "" {
		return ""
	}
	return filepath.Join(dir, name)
}

// fileExists 判断路径是否为存在的常规文件。
func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
