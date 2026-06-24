// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/download"
)

// compile-time interface checks
var (
	_ core.Downloader      = (*DownloaderAdapter)(nil)
	_ core.ContextInjecter = (*DownloaderAdapter)(nil)
	_ core.DomainLimiter   = (*DownloaderAdapter)(nil)
)

// DownloaderAdapter 将 pkg/download.Downloader 包装为 core.Downloader。
// 处理 model.DownloadObject → download.Request 的映射，包括进度转换。
type DownloaderAdapter struct {
	mu              sync.Mutex
	dl              *download.Downloader
	dlCtx           context.Context //nolint:containedctx
	transport       download.Transport
	cancels         sync.Map // map[string]context.CancelFunc
	metrics         *download.MetricRegistry
	metadataFlusher func() // 由 Manager 设置，OnMetadata 触发后调用以立即持久化
}

// NewDownloaderAdapter 创建适配器。
func NewDownloaderAdapter(dl *download.Downloader) *DownloaderAdapter {
	return &DownloaderAdapter{
		dl: dl,
	}
}

// SetMetadataFlusher 设置一个回调，在每次 OnMetadata 写入 obj.Metadata 后调用，
// 用于立即持久化（避免 crash 窗口）。
// 必须在 Download 前调用，不可并发调用。
func (a *DownloaderAdapter) SetMetadataFlusher(fn func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.metadataFlusher = fn
}

// getMetadataFlusher 返回 metadataFlusher（线程安全，与 SetMetadataFlusher 互斥）。
func (a *DownloaderAdapter) getMetadataFlusher() func() {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.metadataFlusher
}

// Name 返回适配器名称（保持与旧 NativeHTTPDownloader 兼容）。
func (a *DownloaderAdapter) Name() string { return "native_http" }

// SetContext 设置下载上下文（替代旧的 NativeHTTPDownloader.SetContext）。
func (a *DownloaderAdapter) SetContext(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dlCtx = ctx
}

// ApplyDomainLimits 设置域名并发限制（通过 StdlibTransport）。
func (a *DownloaderAdapter) ApplyDomainLimits(limits map[string]int) {
	if tr, ok := a.transport.(*download.StdlibTransport); ok {
		tr.SetDomainLimits(limits)
	}
}

// Cancel 尝试取消正在进行的下载。使用 per-URL cancel func 实现。
func (a *DownloaderAdapter) Cancel(url string) error {
	if v, ok := a.cancels.LoadAndDelete(url); ok {
		if cancel, ok := v.(context.CancelFunc); ok {
			cancel()
		}
	}
	return nil
}

// MetricsRegistry returns the download MetricRegistry (implements core.MetricsProvider).
func (a *DownloaderAdapter) MetricsRegistry() any {
	return a.metrics
}

// getCtx 返回当前上下文（线程安全）。
func (a *DownloaderAdapter) getCtx() context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.dlCtx != nil {
		return a.dlCtx
	}
	return context.Background()
}

// copyMetadata 复制 map，使 req.Metadata 不直接引用 obj.Metadata，
// 但保留已有 ETag/checksum 供 http_extractor 的条件请求检查。
func copyMetadata(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	maps.Copy(dst, src)
	return dst
}

// Download 实现 core.Downloader 接口。
// 将 model.DownloadObject + headers 映射为 download.Request 并执行下载。
func (a *DownloaderAdapter) Download(obj *model.DownloadObject, headers map[string]string) error {
	ctx := a.getCtx()

	if headers == nil {
		headers = make(map[string]string)
	}

	// 处理复合下载：检查 Extra["files"]
	// Must lock the object to synchronize with concurrent writes
	// from applySharedState (task/base_task.go).
	obj.RLock()
	filesVal, hasFiles := obj.Extra["files"]
	obj.RUnlock()
	if hasFiles {
		return a.downloadComposite(ctx, obj, headers, filesVal)
	}

	// 标准单文件下载
	req := &download.Request{
		URL:           obj.URL,
		SavePath:      obj.SavePath,
		Headers:       headers,
		Metadata:      func() map[string]string { obj.RLock(); defer obj.RUnlock(); return copyMetadata(obj.Metadata) }(),
		TrackProgress: true,
		OnProgress: func(progress float64, downloaded, total int64) {
			obj.SetProgress(int(progress))
		},
		OnMetadata: func(key, value string) {
			obj.Lock()
			if obj.Metadata == nil {
				obj.Metadata = make(map[string]string)
			}
			obj.Metadata[key] = value
			obj.Unlock()
			if a.getMetadataFlusher() != nil {
				a.getMetadataFlusher()()
			}
		},
	}

	// 创建 per-URL 可取消的 context
	dlCtx, dlCancel := context.WithCancel(ctx)
	defer a.cancels.Delete(obj.URL)
	a.cancels.Store(obj.URL, dlCancel)

	err := a.dl.Download(dlCtx, req)
	if err != nil {
		return fmt.Errorf("adapter: download failed: %w", err)
	}

	// 将 DownloadResult 显式写入 obj.Metadata（加锁保护）
	obj.Lock()
	if r := req.Result; r != nil {
		if r.StatusCode > 0 {
			obj.Metadata["status_code"] = strconv.Itoa(r.StatusCode)
		}
		if r.ContentLength > 0 {
			obj.Metadata["content_length"] = strconv.FormatInt(r.ContentLength, 10)
		}
		if r.TotalSize > 0 {
			obj.Metadata["total_size"] = strconv.FormatInt(r.TotalSize, 10)
		}
		if r.MD5Base64 != "" {
			obj.Metadata["md5_base64"] = r.MD5Base64
		}
		if r.MD5Hex != "" {
			obj.Metadata["md5_hex"] = r.MD5Hex
		}
		if r.ModTime != "" {
			obj.Metadata["mod_time"] = r.ModTime
		}
		// Compatibility shim: set status metadata for consumers not yet migrated
		// to the pkg/download result model. Remove when dlcore is fully removed.
		obj.Metadata["status"] = "completed"
	}
	obj.Unlock()

	obj.SetProgress(100)
	return nil
}

// downloadComposite 处理复合下载（Extra["files"]）。
// 每个子文件按 type 前缀独立保存 ETag/checksum（如 cover_etag、video_checksum 等），
// 并在下载前传入已有的前缀元数据以便断点续传和条件请求。
func (a *DownloaderAdapter) downloadComposite(ctx context.Context, obj *model.DownloadObject, headers map[string]string, filesVal any) error {
	fileList, err := parseCompositeFiles(filesVal)
	if err != nil {
		return err
	}
	// parseCompositeFiles already handles empty list — returns ErrCompositeEmpty

	slog.Info("Starting composite download", "count", len(fileList), "task_id", obj.TaskID)
	for _, fileMap := range fileList {
		subURL := fileMap["url"]
		subPath := fileMap["path"]
		fType := fileMap[model.MetadataKeyType]

		if subURL == "" || subPath == "" {
			continue
		}

		// 确保子文件目录存在
		dir := filepath.Dir(subPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("adapter: failed to create directory for composite file: %w", err)
		}

		// 跟踪进度：仅 video 类型或只有一个文件时
		trackProgress := (fType == "video" || len(fileList) == 1)

		// 为子文件构建带前缀的 metadata，传入已有的 ETag/checksum 供条件请求检查
		subMeta := make(map[string]string)
		prefix := fType + "_"
		obj.RLock()
		if v, ok := obj.Metadata[prefix+"etag"]; ok {
			subMeta["etag"] = v
		}
		if v, ok := obj.Metadata[prefix+"checksum"]; ok {
			subMeta["checksum"] = v
		}
		obj.RUnlock()

		subReq := &download.Request{
			URL:           subURL,
			SavePath:      subPath,
			Headers:       headers,
			Metadata:      subMeta,
			TrackProgress: trackProgress,
			OnProgress: func(progress float64, downloaded, total int64) {
				obj.SetProgress(int(progress))
			},
			OnMetadata: func(key, value string) {
				// 按 type 前缀写入 obj.Metadata（如 cover_etag、video_checksum）
				obj.Lock()
				if obj.Metadata == nil {
					obj.Metadata = make(map[string]string)
				}
				obj.Metadata[prefix+key] = value
				obj.Unlock()
				if a.getMetadataFlusher() != nil {
					a.getMetadataFlusher()()
				}
			},
		}

		if err := a.dl.Download(ctx, subReq); err != nil {
			return fmt.Errorf("adapter: sub-download failed (%s): %w", subURL, err)
		}
	}

	obj.SetProgress(100)
	return nil
}
