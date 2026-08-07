// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"sync"

	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

// compile-time interface check
var _ download.Extractor = (*CompositeExtractor)(nil)
var _ download.TransportSetter = (*CompositeExtractor)(nil)
var _ download.SelectorSetter = (*CompositeExtractor)(nil)
var _ download.Canceller = (*CompositeExtractor)(nil)

// CompositeExtractor 处理复合下载请求。
// 从 req.Metadata["files"] 读取 []map[string]string 格式的文件列表，
// 对每个文件通过注入的 Extractor 执行下载。
type CompositeExtractor struct {
	mu         sync.RWMutex
	selector   download.Selector
	transport  download.Transport
	extractors []download.Extractor
	downloader *download.Downloader
	once       sync.Once
	active     sync.Map // map[string]context.CancelFunc
}

// NewCompositeExtractor 创建 CompositeExtractor 实例。
func NewCompositeExtractor() *CompositeExtractor {
	return &CompositeExtractor{}
}

func (e *CompositeExtractor) Name() string { return "composite" }

// Match 永远返回 false：CompositeExtractor 不参与自动 URL 匹配。
// 它通过任务系统的 metadata["files"] 被动调用，由任务 UI 或调度器直接使用。
// 调用方通过 hint.Extractor == "composite" 显式选择，或直接调用 Extract()。
func (e *CompositeExtractor) Match(ctx context.Context, url string) bool { return false }

func (e *CompositeExtractor) SetSelector(s download.Selector) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.selector = s
}
func (e *CompositeExtractor) SetTransport(t download.Transport) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.transport = t
}

// AddExtractor 向 CompositeExtractor 注册一个 Extractor（用于子下载）。
func (e *CompositeExtractor) AddExtractor(ex download.Extractor) {
	e.extractors = append(e.extractors, ex)
}

// Cancel 取消指定 URL 的子下载。
func (e *CompositeExtractor) Cancel(url string) error {
	if v, ok := e.active.Load(url); ok {
		if cancel, ok := v.(context.CancelFunc); ok {
			cancel()
		}
		e.active.Delete(url)
	}
	return nil
}

// downloadProgress tracks the aggregate progress across sub-downloads.
type downloadProgress struct {
	totalFiles         int
	doneFiles          int
	downloadedBytes    int64
	totalProcessedSize int64
}

// buildDownloader builds or returns the cached downloader instance.
func (e *CompositeExtractor) buildDownloader() *download.Downloader {
	e.once.Do(func() {
		e.mu.RLock()
		transport := e.transport
		selector := e.selector
		extractors := make([]download.Extractor, len(e.extractors))
		copy(extractors, e.extractors)
		e.mu.RUnlock()

		var opts []download.Option
		if transport != nil {
			opts = append(opts, download.WithTransport(transport))
		}
		if selector != nil {
			opts = append(opts, download.WithSelector(selector))
		}
		for _, ex := range extractors {
			opts = append(opts, download.WithExtractor(ex))
		}
		// Always add HTTPExtractor as the last fallback extractor,
		// so HTTP direct links in sub-requests are always handled.
		opts = append(opts, download.WithExtractor(download.NewHTTPExtractor()))
		e.downloader = download.New(opts...)
	})
	return e.downloader
}

// processFile handles a single file entry from the composite file list.
func (e *CompositeExtractor) processFile(ctx context.Context, dl *download.Downloader, fileMap map[string]string, req *download.Request, progress *downloadProgress, fileIndex int, totalFiles int) error {
	subURL := fileMap["url"]
	subPath := fileMap["path"]
	fType := fileMap[model.MetadataKeyType]

	if subURL == "" || subPath == "" {
		return fmt.Errorf("composite: file entry missing url or path at index %d", fileIndex)
	}

	dir := filepath.Dir(subPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("composite: failed to create directory %s: %w", dir, err)
	}

	trackProgress := fType == "video" || totalFiles == 1

	subReq := &download.Request{
		URL:           subURL,
		SavePath:      subPath,
		TrackProgress: trackProgress,
		OnProgress:    req.OnProgress,
		OnMetadata:    req.OnMetadata,
		Headers:       copyMap(req.Headers),
		Metadata:      copyMap(req.Metadata),
		Hint:          req.Hint,
		Result:        &download.DownloadResult{},
	}

	if err := dl.Download(ctx, subReq); err != nil {
		return fmt.Errorf("composite: sub-download failed (%s): %w", subURL, err)
	}

	if info, statErr := os.Stat(subPath); statErr == nil {
		progress.downloadedBytes += info.Size()
		progress.totalProcessedSize += info.Size()
	}
	progress.doneFiles++

	if req.OnProgress != nil && totalFiles > 1 {
		// Dynamic weight progress:
		// pct = (doneFiles / totalFiles) * (downloadedBytes / totalProcessedSize)
		// totalProcessedSize is accumulated from completed files, so byteRatio
		// approaches 1.0 as files complete, making the formula effectively
		// file-count-based but via the dynamic weight code path.
		var pct float64
		if progress.totalProcessedSize > 0 {
			fileRatio := float64(progress.doneFiles) / float64(totalFiles)
			byteRatio := float64(progress.downloadedBytes) / float64(progress.totalProcessedSize)
			pct = fileRatio * byteRatio * 100
		} else {
			pct = float64(progress.doneFiles) / float64(totalFiles) * 100
		}
		req.OnProgress(pct, progress.downloadedBytes, 0)
	}
	return nil
}

// copyMap 深拷贝 map[string]string。
func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string) // 返回空 map，避免 nil map 赋值 panic
	}
	cp := make(map[string]string, len(m))
	maps.Copy(cp, m)
	return cp
}

// parseFiles 从 req.Metadata["files"] 解析文件列表。
// 支持 JSON 字符串 ("[{\"url\":\"...\",\"path\":\"...\",\"type\":\"video\"}]")
func parseFiles(metadata map[string]string) ([]map[string]string, error) {
	filesJSON, ok := metadata["files"]
	if !ok || filesJSON == "" {
		return nil, fmt.Errorf("composite: no 'files' in metadata")
	}
	var fileList []map[string]string
	if err := json.Unmarshal([]byte(filesJSON), &fileList); err != nil {
		return nil, fmt.Errorf("composite: failed to parse files JSON: %w", err)
	}
	if len(fileList) == 0 {
		return nil, fmt.Errorf("composite: files list is empty")
	}
	return fileList, nil
}

// Extract 执行复合下载：
//  1. 从 req.Metadata["files"] 解析文件列表
//  2. 对每个文件，构建子 Request 并调用 Downloader.Download
//  3. 汇总进度
func (e *CompositeExtractor) Extract(ctx context.Context, req *download.Request) error {
	fileList, err := parseFiles(req.Metadata)
	if err != nil {
		return err
	}

	slog.Info("Starting composite download", "count", len(fileList), logutil.LogKeyURL, req.URL)

	dl := e.buildDownloader()
	progress := &downloadProgress{
		totalFiles: len(fileList),
	}

	for i, fileMap := range fileList {
		subURL := fileMap["url"]
		dlCtx, dlCancel := context.WithCancel(ctx)
		e.active.Store(subURL, dlCancel)

		if err := e.processFile(dlCtx, dl, fileMap, req, progress, i, len(fileList)); err != nil {
			e.active.Delete(subURL)
			dlCancel()
			return err
		}
		e.active.Delete(subURL)
		dlCancel()
	}
	if req.Result == nil {
		req.Result = &download.DownloadResult{}
	}
	req.Result.ContentLength = progress.downloadedBytes
	if req.OnProgress != nil {
		req.OnProgress(100, progress.downloadedBytes, progress.downloadedBytes)
	}
	return nil
}
