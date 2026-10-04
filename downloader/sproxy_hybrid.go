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
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

// SproxyHybridDownloader 经 sproxy cloud download API 调用 PikPak 混合下载
// （分享直链前段 + 账号流量后段，分片并行）。
//
// 定位（用户要求）：download-manager 只做「原始任务解析和装配」——不实现 hybrid
// 逻辑（分享签名/转存/账号池/分片），而是把分享 URL（keepshare/mypikpak）交给
// sproxy 的 POST /api/cloud/download（downloaderFor 自动发现 pikpak 下载器，
// hybrid 能力在 sproxy 侧）。dm 侧只负责：解析分享 URL → 提交任务 → 轮询完成 →
// 移动产物到 obj.SavePath。
type SproxyHybridDownloader struct {
	apiURL     string        // sproxy 云下载 API（如 http://127.0.0.1:8080/api/cloud/download）
	apiToken   string        // sproxy API 认证 token（可空）
	pollEvery  time.Duration // 任务轮询间隔
	timeout    time.Duration // 总超时
	httpClient *http.Client
}

// NewSproxyHybridDownloader 创建 sproxy hybrid 下载器。
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
	return &SproxyHybridDownloader{
		apiURL:     strings.TrimRight(apiURL, "/"),
		apiToken:   cfg.APIToken,
		pollEvery:  pollEvery,
		timeout:    timeout,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name 返回下载器名称。
func (d *SproxyHybridDownloader) Name() string { return "sproxy_hybrid" }

// Ensure SproxyHybridDownloader implements core.Downloader
var _ core.Downloader = &SproxyHybridDownloader{}

// Download 把 obj 的分享 URL 提交到 sproxy cloud download，轮询完成后移动产物。
//
// 分享 URL 来源（按优先级）：
//  1. obj.Extra.files 里的 keepshare/mypikpak 分享链接（njavtv magnet_list 分流后）
//  2. obj.URL 本身是分享链接
func (d *SproxyHybridDownloader) Download(obj *model.DownloadObject, headers map[string]string) error {
	shareURL := d.pickShareURL(obj)
	if shareURL == "" {
		return fmt.Errorf("sproxy_hybrid: no share url for %s", obj.URL)
	}

	// 1. 提交任务
	taskID, err := d.submit(shareURL, obj.SavePath)
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

// pickShareURL 从 obj 提取分享 URL（files[0] keepshare 优先，其次 obj.URL）。
func (d *SproxyHybridDownloader) pickShareURL(obj *model.DownloadObject) string {
	if obj == nil {
		return ""
	}
	obj.RLock()
	defer obj.RUnlock()
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
	if isShareURL(obj.URL) {
		return obj.URL
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
func (d *SproxyHybridDownloader) submit(shareURL, savePath string) (string, error) {
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

// poll 轮询 sproxy cloud download 任务状态直到 done/failed/timeout。
func (d *SproxyHybridDownloader) poll(taskID string) error {
	deadline := time.Now().Add(d.timeout)
	statusURL := strings.Replace(d.apiURL, "/download", "/tasks/"+taskID, 1)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s", d.timeout)
		}
		var out struct {
			Status string `json:"status"`
		}
		if err := d.getJSON(statusURL, &out); err != nil {
			// 轮询失败（任务详情 404 等）——重试直到超时
			time.Sleep(d.pollEvery)
			continue
		}
		switch out.Status {
		case "completed", "done":
			return nil
		case "failed", "error":
			return fmt.Errorf("task status %q", out.Status)
		}
		time.Sleep(d.pollEvery)
	}
}

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
