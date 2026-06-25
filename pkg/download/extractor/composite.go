// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

// compile-time interface check
var _ download.Extractor = (*CompositeExtractor)(nil)

// CompositeExtractor 处理复合下载请求。
// 从 req.Metadata["files"] 读取 []map[string]string 格式的文件列表，
// 对每个文件通过注入的 Downloader 执行下载。
type CompositeExtractor struct {
	selector   download.Selector
	transport  download.Transport
	extractors []download.Extractor
	downloader *download.Downloader
}

// NewCompositeExtractor 创建 CompositeExtractor 实例。
func NewCompositeExtractor() *CompositeExtractor {
	return &CompositeExtractor{}
}

func (e *CompositeExtractor) Name() string { return "composite" }

func (e *CompositeExtractor) Match(ctx context.Context, url string) bool { return false }

func (e *CompositeExtractor) SetSelector(s download.Selector)   { e.selector = s }
func (e *CompositeExtractor) SetTransport(t download.Transport) { e.transport = t }

// AddExtractor 向 CompositeExtractor 注册一个 Extractor（用于子下载）。
func (e *CompositeExtractor) AddExtractor(ex download.Extractor) {
	e.extractors = append(e.extractors, ex)
}

// buildDownloader builds or returns the cached downloader instance.
func (e *CompositeExtractor) buildDownloader() *download.Downloader {
	if e.downloader != nil {
		return e.downloader
	}
	var opts []download.Option
	if e.transport != nil {
		opts = append(opts, download.WithTransport(e.transport))
	}
	if e.selector != nil {
		opts = append(opts, download.WithSelector(e.selector))
	}
	for _, ex := range e.extractors {
		opts = append(opts, download.WithExtractor(ex))
	}
	return download.New(opts...)
}

// processFile handles a single file entry from the composite file list.
func (e *CompositeExtractor) processFile(ctx context.Context, dl *download.Downloader, fileMap map[string]string, req *download.Request, totalDownloaded *int64, fileIndex int, totalFiles int) error {
	subURL := fileMap["url"]
	subPath := fileMap["path"]
	fType := fileMap[model.MetadataKeyType]

	if subURL == "" || subPath == "" {
		return nil
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
	}

	if err := dl.Download(ctx, subReq); err != nil {
		return fmt.Errorf("composite: sub-download failed (%s): %w", subURL, err)
	}

	if info, statErr := os.Stat(subPath); statErr == nil {
		*totalDownloaded += info.Size()
	}
	if req.OnProgress != nil && totalFiles > 1 {
		pct := float64(fileIndex+1) / float64(totalFiles) * 100
		req.OnProgress(pct, *totalDownloaded, 0)
	}
	return nil
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

	var totalDownloaded int64
	dl := e.buildDownloader()

	for i, fileMap := range fileList {
		if err := e.processFile(ctx, dl, fileMap, req, &totalDownloaded, i, len(fileList)); err != nil {
			return err
		}
	}

	if req.OnProgress != nil {
		req.OnProgress(100, totalDownloaded, totalDownloaded)
	}
	return nil
}
