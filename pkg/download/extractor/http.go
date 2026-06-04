// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
)

// HTTPExtractor 是通用 HTTP 文件下载编排器。
// 它使用 Transport 做字节传输，自己管理重试、断点续传、MD5 校验。
type HTTPExtractor struct {
	transport  download.Transport
	selector   download.Selector
	maxRetries int
	rootDir    string
	logDir     string
	ua         string
}

// NewHTTPExtractor 创建并返回 HTTPExtractor 实例。
func NewHTTPExtractor() *HTTPExtractor {
	return &HTTPExtractor{
		maxRetries: 5,
		ua:         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
	}
}

// SetTransport 注入 Transport 实例（实现 ExtractorWithTransport 接口）。
func (e *HTTPExtractor) SetTransport(t download.Transport) { e.transport = t }

// SetSelector 注入 Selector 实例（实现 ExtractorWithSelector 接口）。
func (e *HTTPExtractor) SetSelector(s download.Selector) { e.selector = s }

// Name 返回提取器名称。
func (e *HTTPExtractor) Name() string { return "http" }

// Match 判断是否适合处理该 URL。HTTPExtractor 处理非 m3u8 的 URL。
func (e *HTTPExtractor) Match(ctx context.Context, url string) bool {
	return !strings.Contains(strings.ToLower(url), ".m3u8")
}

// Extract 执行完整的 HTTP 文件下载编排。
func (e *HTTPExtractor) Extract(ctx context.Context, req *download.Request) error {
	rPath := req.SavePath
	var err error
	if e.rootDir != "" {
		rPath, err = download.ResolvePath(e.rootDir, req.SavePath)
		if err != nil {
			return err
		}
	}

	// 创建目录
	if err := os.MkdirAll(filepath.Dir(rPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 选择代理
	proxyURL := ""
	if e.selector != nil {
		proxyURL, err = e.selector.SelectProxy(ctx, req.URL, req.Hint)
		if err != nil {
			slog.Warn("Proxy selection failed, falling back to direct", "url", req.URL, "error", err)
		}
	}

	// 检查已有文件大小（断点续传支持）
	startOffset := int64(0)
	if fi, statErr := os.Stat(rPath); statErr == nil && fi.Size() > 0 {
		startOffset = fi.Size()
		slog.Info("Resuming download", "file", req.SavePath, "offset", startOffset)
	}

	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}

	// 重试循环
	for attempt := 1; attempt <= e.maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var success bool
		success, err = e.tryDownload(ctx, rPath, req.URL, proxyURL, startOffset, req)
		if err == nil && success {
			return nil
		}
		if err != nil && download.IsNoTry(err) {
			return err
		}
		if !success && err == nil {
			// 需要重新开始（如 MD5 不匹配或 416）
			startOffset = 0
			continue
		}

		slog.Warn("Download attempt failed, retrying", "attempt", attempt, "url", req.URL, "error", err)
		time.Sleep(time.Duration(attempt) * time.Second)
	}

	return fmt.Errorf("%w: max retries reached (%d)", download.ErrNoTry, e.maxRetries)
}

// tryDownload 执行单次下载尝试。返回 success=true 表示下载完成，否则返回错误。
func (e *HTTPExtractor) tryDownload(ctx context.Context, rPath, rawURL, proxyURL string, startOffset int64, req *download.Request) (bool, error) {
	// 更新状态以供调试
	_ = startOffset

	treq := &download.TransportRequest{
		URL:      rawURL,
		Method:   "GET",
		ProxyURL: proxyURL,
		Headers:  e.buildHeaders(req),
	}

	// 断点续传：设置 Range 头
	if startOffset > 0 {
		treq.Range = &download.RangeRequest{Offset: startOffset}
	}

	tresp, tErr := e.transport.RoundTrip(ctx, treq)
	if tErr != nil {
		return false, tErr
	}
	defer tresp.Body.Close()

	// 检查 HTTP 状态码
	if tresp.StatusCode == http.StatusForbidden || tresp.StatusCode == http.StatusNotFound {
		return false, fmt.Errorf("%w: HTTP %d", download.ErrNoTry, tresp.StatusCode)
	}

	if tresp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		// 416 Range Not Satisfiable → 文件可能已变，重新从 0 开始
		return false, nil
	}

	if tresp.StatusCode != http.StatusOK && tresp.StatusCode != http.StatusPartialContent {
		if tresp.StatusCode >= 400 {
			// 500+ 级错误允许重试（非 ErrNoTry）
			return false, fmt.Errorf("HTTP %d", tresp.StatusCode)
		}
		return false, fmt.Errorf("HTTP error: %d", tresp.StatusCode)
	}

	// 计算总大小
	totalSize := tresp.ContentLength
	if cr := tresp.Headers["Content-Range"]; cr != "" {
		parts := strings.Split(cr, "/")
		if len(parts) == 2 {
			if parsed, pErr := strconv.ParseInt(parts[1], 10, 64); pErr == nil {
				totalSize = parsed
			}
		}
	}
	if startOffset > 0 && totalSize > 0 {
		totalSize += startOffset
	}

	// 写入文件
	fileFlags := os.O_CREATE | os.O_WRONLY
	if startOffset > 0 {
		fileFlags |= os.O_APPEND
	} else {
		fileFlags |= os.O_TRUNC
	}

	file, fErr := os.OpenFile(rPath, fileFlags, 0644)
	if fErr != nil {
		return false, fmt.Errorf("failed to open file: %w", fErr)
	}
	defer file.Close()

	var reader io.Reader = tresp.Body
	if req.TrackProgress && req.OnProgress != nil && totalSize > 0 {
		reader = download.NewProgressReader(tresp.Body, totalSize, req.OnProgress)
	}

	if _, cErr := io.Copy(file, reader); cErr != nil {
		return false, fmt.Errorf("failed to write file: %w", cErr)
	}

	// 填写元数据
	req.Metadata["status_code"] = strconv.Itoa(tresp.StatusCode)
	req.Metadata["content_length"] = strconv.FormatInt(totalSize, 10)

	// MD5 校验
	if wantMd5 := download.TryGetMd5(tresp.Headers); wantMd5 != "" {
		base64MD5, hexMD5, md5Err := download.ComputeFileMD5(rPath)
		if md5Err != nil {
			return false, fmt.Errorf("failed to compute MD5: %w", md5Err)
		}
		if base64MD5 != wantMd5 && hexMD5 != wantMd5 {
			slog.Warn("MD5 mismatch, retrying download", "want", wantMd5, "got", base64MD5)
			return false, nil // return false 触发重新下载
		}
	}

	// 设置 Last-Modified 时间
	if modTimeStr := tresp.Headers["Last-Modified"]; modTimeStr != "" {
		if modTime, pErr := time.Parse(time.RFC1123, modTimeStr); pErr == nil {
			_ = os.Chtimes(rPath, modTime, modTime)
		}
	}

	return true, nil
}

func (e *HTTPExtractor) buildHeaders(req *download.Request) map[string]string {
	h := make(map[string]string)
	if req.Headers != nil {
		for k, v := range req.Headers {
			h[k] = v
		}
	}
	if _, ok := h["User-Agent"]; !ok && e.ua != "" {
		h["User-Agent"] = e.ua
	}
	return h
}