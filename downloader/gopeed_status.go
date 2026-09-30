// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

// GopeedTaskState 一条 Gopeed 下载任务的持久化状态（文件 JSON）。
// 避免静默信息：下载进度/状态/错误在轮询时落盘，重启后可追溯。
type GopeedTaskState struct {
	TaskID     string    `json:"task_id"`
	URL        string    `json:"url"`
	SavePath   string    `json:"save_path"`
	Status     string    `json:"status"` // running/done/error/timeout
	Downloaded int64     `json:"downloaded_bytes"`
	Total      int64     `json:"total_bytes"`
	Speed      int64     `json:"speed_bps"`
	Error      string    `json:"error,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// statusWriter 是 Gopeed 下载状态持久化器（文件 JSON，线程安全）。
type statusWriter struct {
	mu   sync.Mutex
	path string
}

// newStatusWriter 创建状态持久化器；path 为空时禁用（不落盘）。
func newStatusWriter(path string) *statusWriter {
	return &statusWriter{path: path}
}

// write 覆盖写入一条任务状态（保持最新，单任务场景）。
func (w *statusWriter) write(st *GopeedTaskState) {
	if w == nil || w.path == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		slog.Warn("gopeed status: mkdir failed", logutil.LogKeyError, err, "path", w.path)
		return
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		slog.Warn("gopeed status: marshal failed", logutil.LogKeyError, err)
		return
	}
	tmp := w.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		slog.Warn("gopeed status: write failed", logutil.LogKeyError, err, "path", w.path)
		return
	}
	if err := os.Rename(tmp, w.path); err != nil {
		slog.Warn("gopeed status: rename failed", logutil.LogKeyError, err, "path", w.path)
		return
	}
}

// recordTaskState 轮询/终态时持久化任务状态（避免静默）。
func (d *GopeedDownloader) recordTaskState(taskID, url, savePath, status string, downloaded, total, speed int64, errMsg string) {
	if d.status == nil {
		return
	}
	d.status.write(&GopeedTaskState{
		TaskID:     taskID,
		URL:        url,
		SavePath:   savePath,
		Status:     status,
		Downloaded: downloaded,
		Total:      total,
		Speed:      speed,
		Error:      errMsg,
		UpdatedAt:  time.Now(),
	})
}

// recordTaskError 下载失败/超时时记录终态错误。
func (d *GopeedDownloader) recordTaskError(taskID string, obj *model.DownloadObject, status, errMsg string) {
	savePath := ""
	if obj != nil {
		savePath = obj.SavePath
	}
	url := ""
	if obj != nil {
		url = obj.URL
	}
	d.recordTaskState(taskID, url, savePath, status, 0, 0, 0, errMsg)
}
