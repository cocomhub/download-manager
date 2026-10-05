// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
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
	apiURL     string        // sproxy 云下载 API（如 http://127.0.0.1:8080/api/cloud/download）
	apiToken   string        // sproxy API 认证 token（可空；SproxySig 配置后忽略）
	pollEvery  time.Duration // 任务轮询间隔
	timeout    time.Duration // 总超时
	httpClient *http.Client
	sig        *sproxyclient.FileClient // SproxySig 凭据客户端（nil = Bearer 路径）；自带签名+轮换
	sigEnabled bool
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
	sigEnabled := cfg.AccessKey != "" && cfg.AccessKeySecret != ""
	var sig *sproxyclient.FileClient
	if sigEnabled {
		baseURL := strings.TrimSuffix(headerOK, "/api/cloud/download")
		if baseURL == "" {
			baseURL = "http://127.0.0.1:8080"
		}
		opts := []sproxyclient.Option{sproxyclient.WithAccessKey(cfg.AccessKey, cfg.AccessKeySecret), sproxyclient.WithTimeout(clientTimeout(cfg.ClientTimeout))}
		if cfg.AccessKeyID != "" {
			opts = append(opts, sproxyclient.WithAccessKeyID(cfg.AccessKeyID))
		}
		sig = sproxyclient.NewFileClient(baseURL, opts...)
	}
	return &SproxyHybridDownloader{
		apiURL:     strings.TrimRight(apiURL, "/"),
		apiToken:   cfg.APIToken,
		pollEvery:  pollEvery,
		timeout:    timeout,
		httpClient: &http.Client{Timeout: clientTimeout(cfg.ClientTimeout)},
		sig:        sig,
		sigEnabled: sigEnabled,
	}
}

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
	shareURL := d.pickShareURL(obj)
	if shareURL == "" {
		return fmt.Errorf("sproxy_hybrid: no share url for %s", obj.URL)
	}

	// 1. 提交任务（透传 headers；纯 magnet 候选 hybrid 用不了，submit 已只接受分享 URL）
	taskID, err := d.submit(shareURL, obj.SavePath, headers)
	if err != nil {
		return fmt.Errorf("sproxy_hybrid submit %s: %w", shareURL, err)
	}
	slog.Info("Sproxy hybrid task submitted", "task_id", taskID, logutil.LogKeyURL, shareURL)

	// 2. 轮询直到完成
	if err := d.poll(taskID); err != nil {
		return fmt.Errorf("sproxy_hybrid task %s: %w", taskID, err)
	}
	return nil
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
func (d *SproxyHybridDownloader) submitSig(shareURL, savePath string, headers map[string]string) (string, error) {
	body := map[string]string{"url": shareURL}
	if savePath != "" {
		body["filename"] = baseName(savePath)
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
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized {
			// 签名失效（条目过期/删除）：触发轮换后再重试一次
			if rerr := d.rotateOnce(); rerr == nil {
				return d.submitSig(shareURL, savePath, headers)
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

// rotateOnce 调用一次 RenewAccessKey（热替换新 SK；失败 Warn 下轮重试）。
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
	return nil
}

// poll 轮询 sproxy cloud download 任务状态直到 done/failed/timeout。
// 非 2xx 且非 timeout 的错误（404 任务不存在/服务端异常）连续失败 N 次短路返回，
// 避免无限 sleep 到总超时（默认 3h）空等。
// SproxySig 路径：遇 401（签名失效）触发 RenewAccessKey 轮换后重试（pollWithRotate）。
func (d *SproxyHybridDownloader) poll(taskID string) error {
	if d.sigEnabled && d.sig != nil {
		return d.pollWithRotate(taskID)
	}
	const maxConsecutiveErr = 3
	deadline := time.Now().Add(d.timeout)
	statusURL := strings.Replace(d.apiURL, "/download", "/tasks/"+taskID, 1)
	consecErr := 0
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s", d.timeout)
		}
		var out struct {
			Status string `json:"status"`
		}
		if err := d.getJSON(statusURL, &out); err != nil {
			// 轮询失败（404 任务不存在/服务端异常）——连续失败 N 次短路返回，避免 3h 空等
			consecErr++
			if consecErr >= maxConsecutiveErr {
				return fmt.Errorf("task %s poll failed %d consecutive times: %w", taskID, consecErr, err)
			}
			time.Sleep(d.pollEvery)
			continue
		}
		consecErr = 0
		switch out.Status {
		case "completed", "done":
			return nil
		case "failed", "error":
			return fmt.Errorf("task status %q", out.Status)
		}
		time.Sleep(d.pollEvery)
	}
}

// pollWithRotate 用 SproxySig 客户端轮询（GetCloudTask），遇 401（签名失效）先
// RenewAccessKey 轮换（每小时直到成功，每次轮换失败 Warn），成功后用新 SK 继续轮询。
func (d *SproxyHybridDownloader) pollWithRotate(taskID string) error {
	const maxConsecutiveErr = 3
	deadline := time.Now().Add(d.timeout)
	consecErr := 0
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s", d.timeout)
		}
		task, err := d.sig.GetCloudTask(context.Background(), taskID)
		if err != nil {
			// 401 签名失效 → 轮换一次（新 SK 热替换）后重试；其它错误连续短路
			if errors.Is(err, sproxyclient.ErrNotFound) {
				consecErr++
				if consecErr >= maxConsecutiveErr {
					return fmt.Errorf("task %s poll failed %d consecutive times: %w", taskID, consecErr, err)
				}
				time.Sleep(d.pollEvery)
				continue
			}
			// 网络/401 等：尝试轮换
			if rerr := d.rotateOnce(); rerr == nil {
				consecErr = 0
				time.Sleep(d.pollEvery)
				continue
			}
			consecErr++
			if consecErr >= maxConsecutiveErr {
				return fmt.Errorf("task %s poll failed %d consecutive times: %w", taskID, consecErr, err)
			}
			time.Sleep(d.pollEvery)
			continue
		}
		consecErr = 0
		switch task.Status {
		case "completed", "done":
			return nil
		case "failed", "error":
			return fmt.Errorf("task status %q", task.Status)
		}
		time.Sleep(d.pollEvery)
	}
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
