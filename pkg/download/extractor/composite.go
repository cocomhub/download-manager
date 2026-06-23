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
)

// compile-time interface check
var _ download.Extractor = (*CompositeExtractor)(nil)

// CompositeExtractor 澶勭悊澶嶅悎涓嬭浇璇锋眰銆?// 浠?req.Metadata["files"] 璇诲彇 []map[string]string 鏍煎紡鐨勬枃浠跺垪琛紝
// 瀵规瘡涓枃浠堕€氳繃娉ㄥ叆鐨?Downloader 鎵ц涓嬭浇銆?type CompositeExtractor struct {
	selector   download.Selector
	transport  download.Transport
	extractors []download.Extractor
	downloader *download.Downloader
}

// NewCompositeExtractor 鍒涘缓 CompositeExtractor 瀹炰緥銆?func NewCompositeExtractor() *CompositeExtractor {
	return &CompositeExtractor{}
}

func (e *CompositeExtractor) Name() string { return "composite" }

func (e *CompositeExtractor) Match(ctx context.Context, url string) bool { return false }

func (e *CompositeExtractor) SetSelector(s download.Selector)   { e.selector = s }
func (e *CompositeExtractor) SetTransport(t download.Transport) { e.transport = t }

// AddExtractor 鍚?CompositeExtractor 娉ㄥ唽涓€涓?Extractor锛堢敤浜庡瓙涓嬭浇锛夈€?func (e *CompositeExtractor) AddExtractor(ex download.Extractor) {
	e.extractors = append(e.extractors, ex)
}

// parseFiles 浠?req.Metadata["files"] 瑙ｆ瀽鏂囦欢鍒楄〃銆?// 鏀寔 JSON 瀛楃涓?("[{\"url\":\"...\",\"path\":\"...\",\"type\":\"video\"}]")
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

// Extract 鎵ц澶嶅悎涓嬭浇锛?//  1. 浠?req.Metadata["files"] 瑙ｆ瀽鏂囦欢鍒楄〃
//  2. 瀵规瘡涓枃浠讹紝鏋勫缓瀛?Request 骞惰皟鐢?Downloader.Download
//  3. 姹囨€昏繘搴?func (e *CompositeExtractor) Extract(ctx context.Context, req *download.Request) error {
	fileList, err := parseFiles(req.Metadata)
	if err != nil {
		return err
	}

	slog.Info("Starting composite download", "count", len(fileList), "url", req.URL)

	// 鏋勫缓瀛?Downloader锛屾敞鍏?Selector銆乀ransport 鍜?Extractor
	var totalDownloaded int64
	dl := e.downloader
	if dl == nil {
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
		dl = download.New(opts...)
	}

	for i, fileMap := range fileList {
		subURL := fileMap["url"]
		subPath := fileMap["path"]
		fType := fileMap[model.MetadataKeyType]

		if subURL == "" || subPath == "" {
			continue
		}

		// 鍒涘缓鐩綍
		dir := filepath.Dir(subPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("composite: failed to create directory %s: %w", dir, err)
		}

		// 璺熻釜杩涘害锛氫粎 video 绫诲瀷鎴栧彧鏈変竴涓枃浠舵椂
		trackProgress := (fType == "video" || len(fileList) == 1)

		subReq := &download.Request{
			URL:           subURL,
			SavePath:      subPath,
			TrackProgress: trackProgress,
			OnProgress:    req.OnProgress,
		}

		if err := dl.Download(ctx, subReq); err != nil {
			return fmt.Errorf("composite: sub-download failed (%s): %w", subURL, err)
		}

		// 绱宸蹭笅杞藉瓧鑺傛暟
		if info, statErr := os.Stat(subPath); statErr == nil {
			totalDownloaded += info.Size()
		}
		if req.OnProgress != nil && len(fileList) > 1 {
			pct := float64(i+1) / float64(len(fileList)) * 100
			req.OnProgress(pct, totalDownloaded, 0)
		}
	}

	if req.OnProgress != nil {
		req.OnProgress(100, totalDownloaded, totalDownloaded)
	}
	return nil
}
