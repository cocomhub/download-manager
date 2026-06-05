// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/download/transport"
)

// compile-time interface checks
var (
	_ core.Downloader                     = (*DownloaderAdapter)(nil)
	_ core.DownloaderWithContext          = (*DownloaderAdapter)(nil)
	_ core.DownloaderWithDomainLimits     = (*DownloaderAdapter)(nil)
)

// DownloaderAdapter 将 pkg/download.Downloader 包装为 core.Downloader。
// 处理 model.DownloadObject → download.Request 的映射，包括进度转换。
type DownloaderAdapter struct {
	mu        sync.Mutex
	dl        *download.Downloader
	dlCtx     context.Context
	transport download.Transport
	logger    *slog.Logger
}

// NewDownloaderAdapter 创建适配器。
func NewDownloaderAdapter(dl *download.Downloader) *DownloaderAdapter {
	return &DownloaderAdapter{
		dl:     dl,
		logger: slog.Default(),
	}
}

// Name 返回适配器名称。
func (a *DownloaderAdapter) Name() string { return "adapter" }

// SetContext 设置下载上下文（替代旧的 NativeHTTPDownloader.SetContext）。
func (a *DownloaderAdapter) SetContext(ctx context.Context) {
	a.mu.Lock()
	a.dlCtx = ctx
	a.mu.Unlock()
}

// ApplyDomainLimits 设置域名并发限制（通过 StdlibTransport）。
func (a *DownloaderAdapter) ApplyDomainLimits(limits map[string]int) {
	if tr, ok := a.transport.(*transport.StdlibTransport); ok {
		tr.SetDomainLimits(limits)
	}
}

// Download 实现 core.Downloader 接口。
// 将 model.DownloadObject + headers 映射为 download.Request 并执行下载。
func (a *DownloaderAdapter) Download(obj *model.DownloadObject, headers map[string]string) error {
	a.mu.Lock()
	ctx := a.dlCtx
	a.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}

	if headers == nil {
		headers = make(map[string]string)
	}

	// 处理复合下载：检查 Extra["files"]
	if filesVal, ok := obj.Extra["files"]; ok && filesVal != nil {
		return a.downloadComposite(ctx, obj, headers, filesVal)
	}

	// 标准单文件下载
	req := &download.Request{
		URL:      obj.URL,
		SavePath: obj.SavePath,
		Headers:  headers,
		Metadata: obj.Metadata,
		TrackProgress: true,
		OnProgress: func(progress float64, downloaded, total int64) {
			obj.SetProgress(int(progress))
		},
	}

	err := a.dl.Download(ctx, req)
	if err != nil {
		return fmt.Errorf("adapter: download failed: %w", err)
	}
	return nil
}

// downloadComposite 处理复合下载（Extra["files"]）。
func (a *DownloaderAdapter) downloadComposite(ctx context.Context, obj *model.DownloadObject, headers map[string]string, filesVal any) error {
	fileList, err := parseCompositeFiles(filesVal)
	if err != nil {
		return err
	}

	if len(fileList) == 0 {
		return fmt.Errorf("adapter: composite download with empty file list")
	}

	slog.Info("Starting composite download via adapter", "count", len(fileList), "task_id", obj.TaskID)

	for _, fileMap := range fileList {
		subURL := fileMap["url"]
		subPath := fileMap["path"]
		fType := fileMap["type"]

		if subURL == "" || subPath == "" {
			continue
		}

		// 跟踪进度：仅 video 类型或只有一个文件时
		trackProgress := (fType == "video" || len(fileList) == 1)

		subReq := &download.Request{
			URL:           subURL,
			SavePath:      subPath,
			Headers:       headers,
			TrackProgress: trackProgress,
			OnProgress: func(progress float64, downloaded, total int64) {
				obj.SetProgress(int(progress))
			},
		}

		if err := a.dl.Download(ctx, subReq); err != nil {
			return fmt.Errorf("adapter: sub-download failed (%s): %w", subURL, err)
		}
	}

	obj.SetProgress(100)
	return nil
}