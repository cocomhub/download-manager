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
}

// Ensure GopeedDownloader implements core.Downloader
var _ core.Downloader = &GopeedDownloader{}

// gopeedCreateTaskRequest 是 Gopeed POST /api/v1/tasks 的请求体。
type gopeedCreateTaskRequest struct {
	ReqID string              `json:"reqId"`
	Req   gopeedCreateTaskReq `json:"req"`
}

type gopeedCreateTaskReq struct {
	URL      string         `json:"url"`
	Protocol string         `json:"protocol"`
	Extra    map[string]any `json:"extra"`
}

// gopeedResponse 是 Gopeed API 的统一响应包装 {code, message, data}。
type gopeedResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// gopeedTask 是 GET /api/v1/tasks/{id} 返回的任务对象。
type gopeedTask struct {
	ID     string     `json:"id"`
	Status string     `json:"status"` // running / done / error / ...
	Meta   gopeedMeta `json:"meta"`
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
	}
}

func (d *GopeedDownloader) Name() string {
	return "gopeed"
}

// Download 通过 Gopeed 下载 obj.URL 到 obj.SavePath，完成后返回 nil。
func (d *GopeedDownloader) Download(obj *model.DownloadObject, headers map[string]string) error {
	taskID, err := d.createTask(obj)
	if err != nil {
		return fmt.Errorf("gopeed create task: %w", err)
	}

	slog.Info("Gopeed task created", "task_id", taskID, logutil.LogKeyURL, obj.URL)

	deadline := time.Now().Add(d.timeout)
	lastStatus := ""
	for {
		status, err := d.pollTask(taskID)
		if err != nil {
			return fmt.Errorf("gopeed poll task %s: %w", taskID, err)
		}
		lastStatus = status

		switch status {
		case "done":
			slog.Info("Gopeed task done", "task_id", taskID, logutil.LogKeyURL, obj.URL)
			return d.moveResult(obj, taskID)
		case "error":
			return fmt.Errorf("gopeed task %s failed: status=error", taskID)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("gopeed task %s timed out after %s (last status %q)", taskID, d.timeout, lastStatus)
		}
		select {
		case <-time.After(d.pollInterval):
		case <-context.Background().Done():
			return fmt.Errorf("gopeed task %s cancelled", taskID)
		}
	}
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

// pollTask 查询 {rpcURL}/api/v1/tasks/{id} 的任务状态。
func (d *GopeedDownloader) pollTask(taskID string) (string, error) {
	return d.taskStatus(taskID)
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

func (d *GopeedDownloader) taskStatus(taskID string) (string, error) {
	task, err := d.getTask(taskID)
	if err != nil {
		return "", err
	}
	return task.Status, nil
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

// moveResult 把 Gopeed 下载产物移动到 obj.SavePath。
func (d *GopeedDownloader) moveResult(obj *model.DownloadObject, taskID string) error {
	task, err := d.getTask(taskID)
	if err != nil {
		return fmt.Errorf("gopeed get task result: %w", err)
	}

	srcPath := d.resultPath(obj, task.Meta.Res.Files)
	if srcPath == "" {
		return fmt.Errorf("gopeed: unable to resolve downloaded file path for url %s", obj.URL)
	}
	if obj.SavePath == "" {
		return fmt.Errorf("gopeed: obj.SavePath is empty")
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

// resultPath 由任务的产物文件列表解析来源路径；找不到时回退到 URL basename。
func (d *GopeedDownloader) resultPath(obj *model.DownloadObject, files []gopeedFile) string {
	for _, f := range files {
		if f.Name == "" {
			continue
		}
		return filepath.Join(d.downloadDir, f.Name)
	}
	base := filepath.Base(obj.URL)
	return filepath.Join(d.downloadDir, base)
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
