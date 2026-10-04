// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// ErrUnsupportedURL 表示该 URL 不是 Gopeed 下载器可支持的来源（非磁力、非 PikPak
// 分享/直链），不应创建 Gopeed http 任务去下 HTML/页面——上层应回退原生下载器或明确失败。
var ErrUnsupportedURL = errors.New("gopeed: unsupported url scheme for gopeed downloader")

// isMagnetURL 判断 URL 是否为磁力类（magnet:/bt:/ed2k:）。
func isMagnetURL(url string) bool {
	return strings.HasPrefix(url, "magnet:") ||
		strings.HasPrefix(url, "magnetic:") ||
		strings.HasPrefix(url, "bt:") ||
		strings.HasPrefix(url, "ed2k:")
}

// Download 通过 Gopeed 下载 obj.URL 到 obj.SavePath，完成后返回 nil。
//
// 三条路径：
//  1. PikPak 磁力全长（magnet_list 多条候选，或 files[0] keepshare / obj.URL 分享链接）：
//     候选按 size 降序逐个 resolve 免登录直链再交 Gopeed http 下载；
//     任一候选成功即主视频成功，全部失败返回聚合错误（上层回退 HLS）。
//  2. 磁力（magnet:/bt:）：POST /api/v1/tasks 创建磁力任务，轮询直到完成。
//  3. 普通 http(s) 直链：仅当明确来自内部下载路径（PikPak resolve 出的 dl-*.mypikpak.com 直链）
//     才允许——外部手动传入普通 http 详情页/HLS URL 会在入口被拦截，避免 Gopeed 去下页面。
//
// 入口校验：URL 必须为磁力或 PikPak 类，否则返回 ErrUnsupportedURL；
// 但 obj.Extra 含 magnet_list / files 里含 keepshare/magnet（磁力优先已设）时仍允许走 PikPak 分支。
func (d *GopeedDownloader) Download(obj *model.DownloadObject, headers map[string]string) error {
	// 磁力全长优先：obj.Extra.magnet_list 多条磁力候选（keepshare/纯 magnet 按大小降序）
	// 或 files[0] keepshare / obj.URL 分享链接 → downloadViaPikPak；
	// 任一候选成功即主视频成功，全部失败返回聚合错误由上层回退 HLS。
	if d.hasPikPakSource(obj) {
		return d.downloadViaPikPak(obj)
	}
	if !isMagnetURL(obj.URL) {
		err := fmt.Errorf("%w: %s", ErrUnsupportedURL, obj.URL)
		d.recordTaskError("", obj, "error", err.Error())
		return err
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

// hasPikPakSource 判断对象是否有 PikPak 磁力全长下载源：
// files[0] keepshare/magnet 镜像，或 obj.URL 本身就是分享链接。
func (d *GopeedDownloader) hasPikPakSource(obj *model.DownloadObject) bool {
	if obj == nil {
		return false
	}
	return d.hasMagnetList(obj) || d.firstPikPakFromFiles(obj) != "" || d.isPikPakURL(obj.URL)
}

// hasMagnetList 判断对象 Extra 是否含非空 magnet_list。
func (d *GopeedDownloader) hasMagnetList(obj *model.DownloadObject) bool {
	if obj == nil {
		return false
	}
	obj.RLock()
	defer obj.RUnlock()
	if _, ok := obj.Extra["magnet_list"].([]map[string]string); ok {
		return true
	}
	if fa, ok := obj.Extra["magnet_list"].([]any); ok {
		for _, it := range fa {
			if m, ok2 := it.(map[string]any); ok2 {
				if ks, _ := m["keepshare"].(string); ks != "" {
					return true
				}
				if mg, _ := m["magnet"].(string); mg != "" {
					return true
				}
			}
		}
	}
	return false
}

// downloadViaPikPak 处理 PikPak 磁力全长下载：
//  1. 收集候选：obj.Extra.magnet_list 的 keepshare（无则纯 magnet）链接，按 size 降序
//     （无 size 垫底保持 magnet 顺序）；无 magnet_list 时兼容单分享 URL（files[0] 或 obj.URL）。
//  2. 逐个尝试：resolve → 挑目标（dn 匹配/最大 mp4）→ 下载；任一候选成功即主视频成功返回 nil。
//  3. 全部失败：返回聚合错误（含每个候选失败原因），供上层回退 HLS。
func (d *GopeedDownloader) downloadViaPikPak(obj *model.DownloadObject) error {
	if obj == nil {
		msg := "gopeed pikpak: nil object"
		d.recordTaskError("", nil, "error", msg)
		return fmt.Errorf("%s", msg)
	}
	cands := d.collectPikPakCandidates(obj)
	if len(cands) == 0 {
		// 无 magnet_list → 兼容单分享 URL：files[0] keepshare 优先，其次 obj.URL。
		u := d.firstPikPakFromFiles(obj)
		if u == "" {
			u = obj.URL
		}
		if u == "" {
			msg := "gopeed pikpak: no magnet candidate and no share url"
			d.recordTaskError("", obj, "error", msg)
			return fmt.Errorf("%s", msg)
		}
		cands = []magnetCandidate{{name: "", url: u, size: -1}}
	}

	var reasons []string
	for _, cand := range cands {
		if err := d.tryPikPakCandidate(obj, cand); err != nil {
			reasons = append(reasons, fmt.Sprintf("%s: %v", cand.url, err))
			continue
		}
		return nil // 任一候选成功即主视频成功
	}

	if len(cands) == 1 {
		// 单候选：保持原有错误形态（不聚合）。
		msg := reasons[0]
		d.recordTaskError("", obj, "error", msg)
		return fmt.Errorf("%s", msg)
	}
	msg := fmt.Sprintf("gopeed pikpak: all %d magnet candidates failed: %s", len(cands), strings.Join(reasons, "; "))
	d.recordTaskError("", obj, "error", msg)
	return fmt.Errorf("%s", msg)
}

// tryPikPakCandidate 尝试单个磁力候选：keepshare/分享 URL resolve → 挑目标 → 下载。
// 返回 nil 表示该候选成功（主视频已下好）；失败原因返回给调用方继续下一个候选。
func (d *GopeedDownloader) tryPikPakCandidate(obj *model.DownloadObject, cand magnetCandidate) error {
	shareURL := cand.url
	// 纯 magnet 链接直接交给 resolve（Gopeed 按协议分发扩展）；keepshare/分享页先 302 跟随。
	if !strings.HasPrefix(strings.ToLower(cand.url), "magnet:") && !strings.HasPrefix(strings.ToLower(cand.url), "magnetic:") {
		var err error
		shareURL, err = d.resolvePikPakShareURL(cand.url)
		if err != nil {
			return err
		}
	}

	files, err := d.pikpakResolve(shareURL)
	if err != nil {
		return fmt.Errorf("gopeed pikpak resolve %s: %w", shareURL, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("gopeed pikpak resolve %s: no files", shareURL)
	}

	target := d.pickPikPakTarget(files, cand.url)
	if target == nil {
		return fmt.Errorf("gopeed pikpak: no matching file for url %s (resolved %d files)", cand.url, len(files))
	}
	if target.DownloadURL == "" {
		return fmt.Errorf("gopeed pikpak: file %q has no download url", target.Name)
	}

	slog.Info("PikPak resolved target", "candidate", cand.name, "name", target.Name, "size", target.Size, "url", target.DownloadURL[:min(80, len(target.DownloadURL))])

	// 用直链走 Gopeed http 下载；headers（Referer/UA）通过 obj 的下载头透传。
	targetObj := model.DownloadObject{TaskID: obj.TaskID, URL: target.DownloadURL, SavePath: obj.SavePath}
	if err := d.downloadHTTP(&targetObj, target.Headers); err != nil {
		return fmt.Errorf("gopeed pikpak download %s: %w", target.Name, err)
	}
	return nil
}

// magnetCandidate 一个磁力全长候选：keepshare 镜像优先，无 keepshare 时用纯 magnet。
type magnetCandidate struct {
	name string // 磁力名（dn），仅用于日志
	url  string // 待尝试链接：keepshare 镜像或 magnet
	size int64  // 解析后字节数；-1 表示无 size（排序垫底）
}

// collectPikPakCandidates 从 obj.Extra.magnet_list（[{magnet,name,keepshare,rapidgator,size}]）
// 收集磁力候选并按 size 降序排列：keepshare 镜像优先，无 keepshare 时用纯 magnet；
// 无 size（解析失败）垫底并保持 magnet_list 原始顺序（sort.SliceStable）。
func (d *GopeedDownloader) collectPikPakCandidates(obj *model.DownloadObject) []magnetCandidate {
	if obj == nil {
		return nil
	}
	obj.RLock()
	defer obj.RUnlock()
	raw, ok := obj.Extra["magnet_list"].([]map[string]string)
	var items []map[string]string
	switch {
	case ok:
		items = raw
	default:
		if fa, ok2 := obj.Extra["magnet_list"].([]any); ok2 {
			for _, it := range fa {
				if m, ok3 := it.(map[string]any); ok3 {
					item := make(map[string]string, len(m))
					for k, v := range m {
						if s, ok4 := v.(string); ok4 {
							item[k] = s
						}
					}
					items = append(items, item)
				}
			}
		}
	}

	out := make([]magnetCandidate, 0, len(items))
	for _, m := range items {
		u := strings.TrimSpace(m["keepshare"])
		if u == "" {
			u = strings.TrimSpace(m["magnet"])
		}
		if u == "" {
			continue
		}
		size := parseSizeBytes(m["size"])
		if size < 0 {
			// 磁力 URL 内 &size=<bytes> 兜底（无人类可读 size 时）
			if _, after, ok := strings.Cut(u, "&size="); ok {
				if n, err := strconv.ParseInt(strings.Split(after, "&")[0], 10, 64); err == nil {
					size = n
				}
			}
		}
		out = append(out, magnetCandidate{name: m["name"], url: u, size: size})
	}
	const minInt = int64(-1 << 63) // size 无值的占位（垫底）
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := out[i].size, out[j].size
		if si < 0 {
			si = minInt
		}
		if sj < 0 {
			sj = minInt
		}
		return si > sj
	})
	return out
}

// parseSizeBytes 解析人类可读大小（如 "4.65GB"/"768MB"）为字节数；无法解析返回 -1。
func parseSizeBytes(s string) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return -1
	}
	var numStr strings.Builder
	unit := ""
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			numStr.WriteString(string(r))
		case r == '.' || r == ',':
			numStr.WriteString(".")
		default:
			unit += string(r)
		}
	}
	if numStr.String() == "" {
		return -1
	}
	n, err := strconv.ParseFloat(numStr.String(), 64)
	if err != nil {
		return -1
	}
	unit = strings.TrimSpace(unit)
	var mult int64 = 1
	switch {
	case strings.HasPrefix(unit, "TB"):
		mult = 1 << 40
	case strings.HasPrefix(unit, "GB"):
		mult = 1 << 30
	case strings.HasPrefix(unit, "MB"):
		mult = 1 << 20
	case strings.HasPrefix(unit, "KB"):
		mult = 1 << 10
	}
	return int64(n * float64(mult))
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
	// 兜底：优先取最大的视频文件（mp4/mkv/ts/m4v/webm/avi，全长视频通常最大且为主片）。
	var best *pikpakFile
	for i := range files {
		name := strings.ToLower(files[i].Name)
		if !hasVideoExt(name) {
			continue
		}
		if best == nil || files[i].Size > best.Size {
			best = &files[i]
		}
	}
	if best != nil {
		return best
	}
	// 极端兜底：无任何视频扩展名时，若仅一个文件直接接受（磁力分享常为单文件，
	// 文件名可能不含标准扩展，如带编码/点号后缀）；多文件仍返回 nil（避免误下缩略图）。
	if len(files) == 1 {
		return &files[0]
	}
	return nil
}

// hasVideoExt 判断文件名是否带常见视频容器扩展名（全长视频）。
func hasVideoExt(name string) bool {
	for _, ext := range []string{".mp4", ".mkv", ".ts", ".m4v", ".webm", ".avi", ".mov", ".flv", ".wmv", ".m2ts"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
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
