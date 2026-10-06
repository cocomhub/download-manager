// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/logutil"

	sproxyclient "github.com/cocomhub/sproxy/pkg/client"
)

// SproxyHybridDownloader 经 sproxy cloud download API 调用 PikPak 混合下载
// （分享直链前段 + 账号流量后段，分片并行）。
//
// 定位（用户要求）：download-manager 只做「原始任务解析和装配」——不实现 hybrid
// 逻辑（分享签名/转存/账号池/分片），而是把分享 URL（keepshare/mypikpak）交给
// sproxy 的 POST /api/cloud/download（downloaderFor 自动发现 pikpak 下载器，
// hybrid 能力在 sproxy 侧）。dm 侧只负责：解析分享 URL → 提交任务 → 轮询完成 →
// 移动产物到 obj.SavePath。
//
// 认证（item6 SproxySig 接入）：优先 SproxySig 签名认证（AccessKey/SK/skey-id，
// 复用 sproxy pkg/client.FileClient——同 sclient 同一库，自带 v2 签名 + SK 轮换）；
// 未配 SproxySig 三件套时回落旧 Bearer 直连（APIToken，向后兼容零回归）。
type SproxyHybridDownloader struct {
	apiURL      string        // sproxy 云下载 API（如 http://127.0.0.1:8080/api/cloud/download）
	apiToken    string        // sproxy API 认证 token（可空；SproxySig 配置后忽略）
	pollEvery   time.Duration // 任务轮询间隔
	timeout     time.Duration // 总超时
	httpClient  *http.Client
	sig         *sproxyclient.FileClient // SproxySig 凭据客户端（nil = Bearer 路径）；自带签名+轮换
	sigEnabled  bool
	ak          string // SproxySig AccessKey（启动验证 ListAccessKeys 用）
	skid        string // SproxySig SK 条目 ID（启动验证取过期时间用）
	transferVol string // 转存目标卷（非空 → 提交带 transfer；默认留 cloud 桶）
	pullBack    bool   // 可选补拉回 SavePath（默认 false=只转存不下载）
	// 主动 SK 轮换调度（提前 24h 每小时直到成功）：
	now        func() time.Time // 时钟注入（测试可控）
	rotateMu   sync.Mutex
	expireAt   time.Time // 当前 SK 过期时间（RenewAccessKey/启动验证 记录）
	lastRotate time.Time // 上次轮换时刻（每小时限频）
	verified   bool      // 启动验证通过（签名链路有效才启用 SproxySig 路径；否则 degraded）
}

// NewSproxyHybridDownloader 创建 sproxy hybrid 下载器。
// 配置了 access_key/access_key_secret → 用 FileClient（SproxySig 签名认证）；否则旧 Bearer。
func NewSproxyHybridDownloader(cfg config.SproxyHybridConfig) *SproxyHybridDownloader {
	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = "http://127.0.0.1:8080/api/cloud/download"
	}
	pollEvery := cfg.PollEvery
	if pollEvery <= 0 {
		pollEvery = 5 * time.Second
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Hour
	}
	headerOK := strings.TrimRight(apiURL, "/")
	// 评审 P0-1：SproxySig 需三件套齐（AK/SK/ID）才启用——缺 access_key_id 时 v2 签名
	// 必被服务端 401，半启用会造成「看似配好、实则全任务失败」静默失效。
	sigEnabled := cfg.AccessKey != "" && cfg.AccessKeySecret != "" && cfg.AccessKeyID != ""
	var sig *sproxyclient.FileClient
	if sigEnabled {
		baseURL := strings.TrimSuffix(headerOK, "/api/cloud/download")
		if baseURL == "" {
			baseURL = "http://127.0.0.1:8080"
		}
		opts := []sproxyclient.Option{
			sproxyclient.WithAccessKey(cfg.AccessKey, cfg.AccessKeySecret),
			sproxyclient.WithAccessKeyID(cfg.AccessKeyID),
			sproxyclient.WithTimeout(clientTimeout(cfg.ClientTimeout)),
		}
		sig = sproxyclient.NewFileClient(baseURL, opts...)
	}
	d := &SproxyHybridDownloader{
		apiURL:      strings.TrimRight(apiURL, "/"),
		apiToken:    cfg.APIToken,
		pollEvery:   pollEvery,
		timeout:     timeout,
		httpClient:  &http.Client{Timeout: clientTimeout(cfg.ClientTimeout)},
		sig:         sig,
		sigEnabled:  sigEnabled,
		ak:          cfg.AccessKey,
		skid:        cfg.AccessKeyID,
		transferVol: cfg.TransferVolume,
		pullBack:    cfg.PullBackToSavePath,
		now:         time.Now,
	}
	// 用户要求：启动时立刻确认签名链路有效才启用——同步带外验证（ListAccessKeys 200），
	// 失败仅 Warn + 标记 degraded（Download 显式报错，不静默降级）；成功则预热 expireAt
	// 使「过期前 24h 每小时轮换」调度立即生效。
	if sigEnabled {
		d.verifyOnStart()
	}
	return d
}

// verifyOnStart 启动时验证 SproxySig 签名链路有效：ListAccessKeys 返回 200 即凭据
// 有效（服务端真实鉴权）；成功时从 SK 列表取当前条目过期时间预热 expireAt（供提前
// 24h 每小时轮换调度）。失败 → verified=false（Download 显式报错，不静默降级）。
func (d *SproxyHybridDownloader) verifyOnStart() {
	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout(0))
	defer cancel()
	infos, err := d.sig.ListAccessKeys(ctx, d.ak)
	if err != nil {
		slog.Error("sproxy sig verify-on-start failed: 签名链路不可用，SproxySig 路径停用（任务将显式失败）",
			"err", err, "ak", d.ak)
		d.verified = false
		return
	}
	d.verified = true
	// 预热 expireAt：找当前 skeyID 条目过期；找不到取最晚过期（服务端多 SK 共存）
	var latest time.Time
	for _, info := range infos {
		if info.SKID == d.skid {
			d.expireAt = info.Expires
			break
		}
		if info.Expires.After(latest) {
			latest = info.Expires
		}
	}
	if d.expireAt.IsZero() {
		d.expireAt = latest
	}
	d.lastRotate = d.now()
	slog.Info("sproxy sig verified on start", "ak", d.ak, "expire_at", d.expireAt.Format(time.RFC3339))
}

// 主动 SK 轮换调度（用户确认策略：拿到凭证记录过期时间，提前 24h 开始每小时轮换直到成功）。
const (
	rotateAhead       = 24 * time.Hour // 到期前 24h 进入轮换窗口
	rotateIntervalGap = time.Hour      // 相邻两次轮换的最小间隔（每小时限频）
)

// clientTimeout 返回单请求超时：0 或负用默认 30s（可由 ClientTimeout 放宽）。
func clientTimeout(ct time.Duration) time.Duration {
	if ct <= 0 {
		return 30 * time.Second
	}
	return ct
}

// Name 返回下载器名称。
func (d *SproxyHybridDownloader) Name() string { return "sproxy_hybrid" }

// Ensure SproxyHybridDownloader implements core.Downloader
var _ core.Downloader = &SproxyHybridDownloader{}

// Download 把 obj 的分享 URL 提交到 sproxy cloud download，轮询完成后移动产物。
//
// headers 透传给 submit（sproxy 若需 Referer/UA 等下载头，不丢失）。
//
// 分享 URL 来源（按优先级）：
//  1. obj.Extra.magnet_list 里的 keepshare 分享链接（与 gopeed collectPikPakCandidates 对齐）
//  2. obj.Extra.files 里的 keepshare/mypikpak 分享链接
//  3. obj.URL 本身是分享链接
func (d *SproxyHybridDownloader) Download(obj *model.DownloadObject, headers map[string]string) error {
	// 用户要求：启动验证失败（签名链路不可用）→ 显式拒绝任务，不静默降级
	if d.sigEnabled && !d.verified {
		return fmt.Errorf("sproxy_hybrid: SproxySig 启动验证失败（签名链路不可用），任务拒绝；请检查 access_key/access_key_secret/access_key_id 配置")
	}
	shareURL := d.pickShareURL(obj)
	if shareURL == "" {
		return fmt.Errorf("sproxy_hybrid: no share url for %s", obj.URL)
	}

	// 1. 提交前：SproxySig 凭证到期前 24h 窗口内先主动轮换（每小时限频，失败不阻塞提交）
	if d.sigEnabled {
		_ = d.ensureRotatedBeforeSubmit()
	}
	// 1. 提交任务（透传 headers；纯 magnet 候选 hybrid 用不了，submit 已只接受分享 URL）
	taskID, err := d.submit(shareURL, obj.SavePath, headers)
	if err != nil {
		return fmt.Errorf("sproxy_hybrid submit %s: %w", shareURL, err)
	}
	slog.Info("Sproxy hybrid task submitted", "task_id", taskID, logutil.LogKeyURL, shareURL)

	// 2. 轮询直到完成（SproxySig 路径返回 CloudTask 供转存 URL 记录/拉回）
	task, err := d.pollResult(taskID)
	if err != nil {
		return fmt.Errorf("sproxy_hybrid task %s: %w", taskID, err)
	}
	// 3. 转存 URL 记录到 obj.Extra（默认转存；不下载）
	if task != nil && task.TransferURL != "" {
		obj.Lock()
		if obj.Extra == nil {
			obj.Extra = make(map[string]any)
		}
		obj.Extra["transfer_url"] = task.TransferURL
		obj.Unlock()
	}
	// 4. 可选补拉回：PullBackToSavePath=true → 用 kind=cloud_task 下载原始文件到 SavePath
	if task != nil && d.pullBack && d.sig != nil && task.Filename != "" {
		if perr := d.pullBackToSavePath(task, obj.SavePath); perr != nil {
			return fmt.Errorf("sproxy_hybrid pullback %s: %w", task.ID, perr)
		}
	}
	return nil
}

// pullBackToSavePath 用 FileClient 以 kind=cloud_task 下载任务原始文件到本地 SavePath。
func (d *SproxyHybridDownloader) pullBackToSavePath(task *sproxyclient.CloudTask, savePath string) error {
	if savePath == "" {
		// 无本地目标（submit 未传 filename）→ 只记录 transfer_url，不拉回
		return nil
	}
	// 服务端 cloud_task 下载 filename=<taskID>/<file>（resolveCloudTaskPath 契约）
	return d.sig.ChunkedDownload(context.Background(), task.ID+"/"+task.Filename, savePath,
		sproxyclient.WithChunkedKind(sproxyclient.DownloadKindCloudTask))
}

// pickShareURL 从 obj 提取分享 URL（按优先级，与 gopeed collectPikPakCandidates 对齐）：
//  1. obj.Extra.magnet_list[].keepshare 分享链接（keepshare 镜像 = HTTP 形态，302→分享页，hybrid 可用）
//  2. obj.Extra.files 里的 keepshare/mypikpak 分享链接
//  3. obj.URL 本身是分享链接
//
// 注意：magnet_list 里的纯 magnet（`magnet:`/`bt:`）走 Gopeed P2P 分支，hybrid resolve 不回
// 分享直链 → 不采集（正确忽略，spx not support 纯 magnet）。
func (d *SproxyHybridDownloader) pickShareURL(obj *model.DownloadObject) string {
	if obj == nil {
		return ""
	}
	obj.RLock()
	defer obj.RUnlock()
	// ① magnet_list[].keepshare 优先（与 gopeed 同源解析）：keepshare 分享直链 hybrid 可用。
	if u := d.shareFromMagnetList(obj); u != "" {
		return u
	}
	// ② files[] 分享链接
	if u := d.shareFromFiles(obj); u != "" {
		return u
	}
	// ③ obj.URL 本身
	if isShareURL(obj.URL) {
		return obj.URL
	}
	return ""
}

// shareFromMagnetList 从 magnet_list 取第一个 keepshare 分享直链（keepshare.org/<id>/magnet: 形态）。
func (d *SproxyHybridDownloader) shareFromMagnetList(obj *model.DownloadObject) string {
	raw, ok := obj.Extra["magnet_list"].([]map[string]string)
	if ok {
		for _, m := range raw {
			if u := strings.TrimSpace(m["keepshare"]); u != "" && isShareURL(u) {
				return u
			}
		}
		return ""
	}
	if fa, ok := obj.Extra["magnet_list"].([]any); ok {
		for _, it := range fa {
			if m, ok2 := it.(map[string]any); ok2 {
				if u, _ := m["keepshare"].(string); u != "" && isShareURL(u) {
					return u
				}
			}
		}
	}
	return ""
}

// shareFromFiles 从 obj.Extra.files 取第一个分享链接（keepshare/mypikpak）。
func (d *SproxyHybridDownloader) shareFromFiles(obj *model.DownloadObject) string {
	if raw, ok := obj.Extra["files"].([]map[string]string); ok {
		for _, f := range raw {
			if u := strings.TrimSpace(f["url"]); u != "" && isShareURL(u) {
				return u
			}
		}
	}
	if fa, ok := obj.Extra["files"].([]any); ok {
		for _, it := range fa {
			if fm, ok := it.(map[string]any); ok {
				if u, _ := fm["url"].(string); u != "" && isShareURL(u) {
					return u
				}
			}
		}
	}
	return ""
}

// isShareURL 判断是否为 PikPak 分享 URL（keepshare / mypikpak / mypikpak.net）。
func isShareURL(u string) bool {
	low := strings.ToLower(u)
	return strings.Contains(low, "keepshare.org/") ||
		strings.Contains(low, "keepshare.cc/") ||
		strings.Contains(low, "mypikpak.com/s/") ||
		strings.Contains(low, "mypikpak.net/s/")
}

// submit POST 到 sproxy cloud download，返回 task id。
// headers 透传给请求（sproxy 若需 Referer/UA 等下载头不丢失）。
// SproxySig 路径用 FileClient（RequestRaw 透传 headers + 签名）；Bearer 路径走 HTTP 直连。
func (d *SproxyHybridDownloader) submit(shareURL, savePath string, headers map[string]string) (string, error) {
	if d.sigEnabled && d.sig != nil {
		return d.submitSig(shareURL, savePath, headers)
	}
	body := map[string]string{"url": shareURL}
	if savePath != "" {
		body["filename"] = baseName(savePath)
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, d.apiURL, strings.NewReader(string(b)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		if k == "" || v == "" {
			continue
		}
		req.Header.Set(k, v)
	}
	if d.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiToken)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateSproxy(string(respBody), 200))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("decode submit response: %w", err)
	}
	if out.ID == "" {
		return "", fmt.Errorf("submit response missing task id")
	}
	return out.ID, nil
}

// submitSig 用 SproxySig 客户端提交（RequestRaw 带签名 + 透传自定义 headers）。
// TransferVolume 非空 → 请求体带 transfer（master 新三行为：转存到卷）。
// 401（签名失效）触发轮换后**有限次重试**（评审 P1-1：无界递归 → DoS）；
// 重试次数耗尽后返回显式错误（不静默降级）。
func (d *SproxyHybridDownloader) submitSig(shareURL, savePath string, headers map[string]string) (string, error) {
	const maxSubmitRetry = 2
	for attempt := 0; attempt <= maxSubmitRetry; attempt++ {
		body := map[string]any{"url": shareURL}
		if savePath != "" {
			body["filename"] = baseName(savePath)
		}
		if d.transferVol != "" {
			body["transfer"] = map[string]any{"volume": d.transferVol}
		}
		b, _ := json.Marshal(body)
		hdr := make(http.Header)
		for k, v := range headers {
			if k == "" || v == "" {
				continue
			}
			hdr.Set(k, v)
		}
		resp, err := d.sig.RequestRaw(context.Background(), http.MethodPost, "/api/cloud/download", strings.NewReader(string(b)), hdr)
		if err != nil {
			return "", err
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			// 仅 401 触发轮换重试（有限次）；其它错误直接返回
			if resp.StatusCode == http.StatusUnauthorized && attempt < maxSubmitRetry {
				if rerr := d.rotateOnce(); rerr == nil {
					continue // 换新 SK 后重试
				}
			}
			return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateSproxy(string(respBody), 200))
		}
		var out struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(respBody, &out); err != nil {
			return "", fmt.Errorf("decode submit response: %w", err)
		}
		if out.ID == "" {
			return "", fmt.Errorf("submit response missing task id")
		}
		return out.ID, nil
	}
	return "", fmt.Errorf("submit failed after %d attempts", maxSubmitRetry+1)
}

// rotateOnce 调用一次 RenewAccessKey（热替换新 SK；失败 Warn 下轮重试）。
// 成功后记录新 SK 的过期时间（供提前 24h 主动轮换调度）。
func (d *SproxyHybridDownloader) rotateOnce() error {
	if d.sig == nil {
		return fmt.Errorf("sproxy sig client not initialized")
	}
	res, err := d.sig.RenewAccessKey(context.Background())
	if err != nil {
		slog.Warn("sproxy sig renew failed", "err", err)
		return err
	}
	slog.Info("sproxy sig key rotated", "sk_id", res.SKID, "expires", res.ExpiresAt)
	d.recordRotate(res)
	return nil
}

// recordRotate 记录轮换结果：新 SK 过期时间 + 本次轮换时刻（限频基准）。
func (d *SproxyHybridDownloader) recordRotate(res *sproxyclient.RenewResult) {
	now := d.now()
	d.rotateMu.Lock()
	if !res.ExpiresAt.IsZero() {
		d.expireAt = res.ExpiresAt
	}
	d.lastRotate = now
	d.rotateMu.Unlock()
}

// ensureRotatedBeforeSubmit 提交前检查：若凭证进入「到期前 24h 窗口」且距上次轮换
// >1h，触发一次轮换（直到成功；失败仅 Warn，下个任务/下小时重试——不阻塞提交）。
// 用于推进「提前 24h 每小时轮换直到成功」的调度。
func (d *SproxyHybridDownloader) ensureRotatedBeforeSubmit() error {
	if d.sig == nil {
		return nil
	}
	d.rotateMu.Lock()
	expireAt := d.expireAt
	lastRotate := d.lastRotate
	d.rotateMu.Unlock()
	now := d.now()
	// 未记录到期时间（尚未 renew 过）→ 无可调度；等 401 触发
	if expireAt.IsZero() {
		return nil
	}
	// 距到期 >24h → 不轮换
	if now.Add(rotateAhead).Before(expireAt) {
		return nil
	}
	// 距上次轮换 <1h → 限频跳过
	if !lastRotate.IsZero() && now.Sub(lastRotate) < rotateIntervalGap {
		return nil
	}
	// 进窗口：每小时轮换直到成功（失败返错，由调用方决定是否继续）
	return d.rotateOnce()
}

// poll 轮询 sproxy cloud download 任务状态直到 done/failed/timeout。
// 非 2xx 且非 timeout 的错误（404 任务不存在/服务端异常）连续失败 N 次短路返回，
// 避免无限 sleep 到总超时（默认 3h）空等。
// SproxySig 路径：遇 401（签名失效）触发 RenewAccessKey 轮换后重试（pollWithRotate）。
func (d *SproxyHybridDownloader) poll(taskID string) error {
	_, err := d.pollResult(taskID)
	return err
}

// pollResult 轮询直到完成，返回最终 CloudTask（含 transfer_url / filename 供转存记录与拉回）。
// SproxySig 路径：遇 401（签名失效）触发 RenewAccessKey 轮换后重试（pollWithRotate）。
func (d *SproxyHybridDownloader) pollResult(taskID string) (*sproxyclient.CloudTask, error) {
	if d.sigEnabled && d.sig != nil {
		return d.pollWithRotateResult(taskID)
	}
	const maxConsecutiveErr = 3
	deadline := time.Now().Add(d.timeout)
	statusURL := strings.Replace(d.apiURL, "/download", "/tasks/"+taskID, 1)
	consecErr := 0
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out after %s", d.timeout)
		}
		var out struct {
			Status      string `json:"status"`
			TransferURL string `json:"transfer_url"`
			Filename    string `json:"filename"`
		}
		if err := d.getJSON(statusURL, &out); err != nil {
			// 轮询失败（404 任务不存在/服务端异常）——连续失败 N 次短路返回，避免 3h 空等
			consecErr++
			if consecErr >= maxConsecutiveErr {
				return nil, fmt.Errorf("task %s poll failed %d consecutive times: %w", taskID, consecErr, err)
			}
			time.Sleep(d.pollEvery)
			continue
		}
		consecErr = 0
		switch out.Status {
		case "completed", "done":
			return &sproxyclient.CloudTask{ID: taskID, Status: out.Status, TransferURL: out.TransferURL, Filename: out.Filename}, nil
		case "failed", "error", "cancelled":
			return nil, fmt.Errorf("task status %q", out.Status)
		}
		time.Sleep(d.pollEvery)
	}
}

// pollWithRotateResult 用 SproxySig 客户端轮询（GetCloudTask），**仅 401**（签名失效）
// 才触发 RenewAccessKey 轮换（评审 P1-A/P1-2：5xx/网络错误不轮换，防凭据风暴）；
// 轮换成功后用新 SK 继续轮询，连续错误仍计数短路。
func (d *SproxyHybridDownloader) pollWithRotateResult(taskID string) (*sproxyclient.CloudTask, error) {
	const maxConsecutiveErr = 3
	deadline := time.Now().Add(d.timeout)
	consecErr := 0
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out after %s", d.timeout)
		}
		task, err := d.sig.GetCloudTask(context.Background(), taskID)
		if err != nil {
			// 404（任务不存在）→ 连续计数短路；其它（500/网络）同样计数——不轮换
			if !isUnauthorizedErr(err) {
				consecErr++
				if consecErr >= maxConsecutiveErr {
					return nil, fmt.Errorf("task %s poll failed %d consecutive times: %w", taskID, consecErr, err)
				}
				time.Sleep(d.pollEvery)
				continue
			}
			// 仅 401：轮换一次（新 SK 热替换）后重试；轮换失败也计数（不风暴）
			if rerr := d.rotateOnce(); rerr == nil {
				time.Sleep(d.pollEvery)
				continue
			}
			consecErr++
			if consecErr >= maxConsecutiveErr {
				return nil, fmt.Errorf("task %s poll failed %d consecutive times: %w", taskID, consecErr, err)
			}
			time.Sleep(d.pollEvery)
			continue
		}
		consecErr = 0
		switch task.Status {
		case "completed", "done":
			return task, nil
		case "failed", "error", "cancelled":
			return nil, fmt.Errorf("task status %q", task.Status)
		}
		time.Sleep(d.pollEvery)
	}
}

// isUnauthorizedErr 判断错误是否为 401（签名失效）。优先 errors.Is(ErrUnauthorized)
// 哨兵（sproxy master 已支持）；哨兵未合并时回退字符串匹配（旧伪版本兼容）。
// 注意：不能静态引用 sproxyclient.ErrUnauthorized（旧伪版本无此常量会编译失败），
// 用 errors.New 本地哨兵 + errors.Is 无法跨包命中 → 直接字符串判断（401 特征稳定）。
func isUnauthorizedErr(err error) bool {
	if err == nil {
		return false
	}
	// HTTP 401 特征：sproxy doJSONStatusError 文案固定为 "请求失败 (HTTP 401)"
	return strings.Contains(err.Error(), "(HTTP 401)")
}

// apiBase 返回 sproxy 服务基地址（apiURL 去掉 /api/cloud/download）。
// getJSON GET 请求并解析 JSON。
func (d *SproxyHybridDownloader) getJSON(url string, out any) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if d.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiToken)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateSproxy(string(b), 200))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// baseName 返回路径的文件名（不含目录）。
func baseName(p string) string {
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// truncateSproxy 截断字符串。
func truncateSproxy(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
