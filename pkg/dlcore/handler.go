// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dlcore

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Handler 定义 URL 匹配与下载能力。
// 实现方通过 Match 判定是否能处理该 URL，通过 Download 执行下载。
type Handler interface {
	// Match 判断此 Handler 是否能处理该 URL
	Match(url string) bool
	// Download 执行下载
	Download(ctx context.Context, req *Request) error
	// Name 返回处理器名称（用于日志/监控）
	Name() string
}

// HandlerWithClient 表示 Handler 需要在初始化后注入 Client 引用。
// 当 Handler 需要访问 Client 配置时（代理、日志路径等）应实现此接口。
type HandlerWithClient interface {
	SetClient(*Client)
}

// registeredHandler 已注册的 handler 条目
type registeredHandler struct {
	name    string
	handler Handler
}

// handlers 全局注册表，按注册顺序存储
var handlers []registeredHandler

func init() {
	RegisterHandler("ffmpeg", &ffmpegHandler{})
}

// RegisterHandler 注册 Handler 到全局注册表。
// 后注册的 handler 匹配优先级更高（插入到列表头部）。
// 由各 handler 的 init() 函数调用。
func RegisterHandler(name string, h Handler) {
	handlers = append([]registeredHandler{{name: name, handler: h}}, handlers...)
}

// matchHandler 遍历全局注册表，返回第一个 Match(url) 为 true 的 Handler。
// 没有任何 handler 匹配时返回 nil。
func matchHandler(url string) Handler {
	for _, rh := range handlers {
		if rh.handler.Match(url) {
			return rh.handler
		}
	}
	return nil
}

// ---- HTTP 默认下载处理器 ----

// httpHandler 是默认的 HTTP 下载处理器，处理常规文件下载。
// Match 始终返回 false，仅在无其他 handler 匹配时作为兜底使用。
type httpHandler struct {
	client *Client
}

func (h *httpHandler) Match(url string) bool { return false }
func (h *httpHandler) Name() string          { return "http" }
func (h *httpHandler) SetClient(c *Client)   { h.client = c }

func (h *httpHandler) Download(ctx context.Context, req *Request) error {
	c := h.client
	rPath := req.SavePath
	var err error
	if c.rootDir != "" {
		rPath, err = ResolvePath(c.rootDir, req.SavePath)
		if err != nil {
			return err
		}
	}

	// 确保目录存在
	dir := filepath.Dir(rPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 准备日志文件
	var logFile string
	var f io.Writer = io.Discard
	if c.logDir != "" {
		logFileName := filepath.Base(rPath)
		if strings.HasPrefix(logFileName, "0") {
			s := strings.Split(rPath, "/")
			if len(s) > 2 {
				logFileName = s[len(s)-2] + " -- " + s[len(s)-1]
			}
		}
		logFile = filepath.Join(c.logDir, logFileName+"."+
			time.Now().Format("20060102150405")+".native.log")
		ff, err := os.Create(logFile)
		if err != nil {
			slog.Warn("Failed to create log file", "file", logFile, "error", err)
		} else {
			defer ff.Close()
			f = ff
		}
	}

	fmt.Fprintf(f, "\n\nSave file to %s\n\n", rPath)

	// 确定代理设置
	proxyURL := ""
	if c.proxySelector != nil {
		proxyURL, err = c.determineProxy(req)
		if err != nil {
			slog.Warn("Proxy selection failed, falling back to direct",
				"url", req.URL, "error", err)
		}
	}

	urlStr := req.URL
	client := c.client
	if proxyURL != "" {
		urlStr = strings.TrimPrefix(urlStr, "http://")
		urlStr = strings.TrimPrefix(urlStr, "https://")
		urlStr = proxyURL + "/" + urlStr
		slog.Info("Using proxy", "url", urlStr, "proxy", proxyURL)
	}

	// 检查文件是否存在以支持断点续传
	var startOffset int64 = 0
	fileInfo, err := os.Stat(rPath)
	if err == nil && fileInfo.Size() > 0 {
		startOffset = fileInfo.Size()
		slog.Info("Resuming download", "file", req.SavePath, "offset", startOffset)
	}

	defer func() {
		if err != nil {
			fmt.Fprintf(f, "%s Download failed: %v\n", time.Now().Format(time.RFC3339Nano), err)
		}
	}()

	cnt := 0
startDownload:
	if c.maxRetries != 0 && cnt >= c.maxRetries {
		fmt.Fprintf(f, "Max retries reached: %d\n", c.maxRetries)
		return fmt.Errorf("max retries reached: %d", c.maxRetries)
	}
	cnt++

	fmt.Fprintf(f, "Requesting URL: %s (Attempt %d)\n\n", urlStr, cnt)
	c.dLimiter.Acquire(req.URL)
	slog.Info("Starting download", "downloader", "dlcore",
		"url", urlStr, "path", req.SavePath, "log", logFile, "attempt", cnt)

	dctx, cancel := context.WithCancel(ctx)
	c.active.Store(req.URL, cancel)

	cleanupDL := func() {
		c.active.Delete(req.URL)
		cancel()
		c.dLimiter.Release(req.URL)
	}

	hreq, err := http.NewRequestWithContext(dctx, "GET", urlStr, nil)
	if err != nil {
		cleanupDL()
		return fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头模拟浏览器行为
	c.addBrowserLikeHeaders(req, hreq)

	var resp *http.Response
	// 设置 Range 头支持断点续传
	if startOffset > 0 {
		nreq := hreq.Clone(dctx)
		printRequestHeaders(f, nreq)
		resp, err = client.Do(nreq)
		if resp != nil && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound) {
			cleanupDL()
			return fmt.Errorf("%w: HTTP %d", ErrNoTry, resp.StatusCode)
		}
		if err != nil {
			cleanupDL()
			return fmt.Errorf("HTTP request failed: %w", err)
		}
		defer resp.Body.Close()
		printResponseHeaders(f, resp)

		contentLength, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		if err == nil && (contentLength == startOffset || contentLength == 0 || contentLength == -1 || resp.ContentLength == startOffset) {
			wantMd5 := TryGetMd5(resp)
			if wantMd5 == "" {
				fmt.Fprintf(f, "The file is already fully retrieved; nothing to do.")
				cleanupDL()
				return nil
			}
			base64MD5, hexMD5, err := computeFileMD5(rPath)
			if err != nil {
				cleanupDL()
				return fmt.Errorf("failed to compute file MD5: %w", err)
			}
			if base64MD5 == wantMd5 || hexMD5 == wantMd5 {
				fmt.Fprintf(f, "MD5 check passed: %s (hex: %s)\n", base64MD5, hexMD5)
				req.Metadata["md5_base64"] = base64MD5
				req.Metadata["md5_hex"] = hexMD5
				modTimeStr := resp.Header.Get("Last-Modified")
				if modTimeStr != "" {
					modTime, err := time.Parse(time.RFC1123, modTimeStr)
					if err == nil {
						os.Chtimes(rPath, modTime, modTime)
					}
					req.Metadata["mod_time"] = modTime.Format(time.RFC3339Nano)
				}
				req.Metadata["total_size"] = strconv.FormatInt(contentLength, 10)
				req.Metadata["status"] = StatusCompleted
				cleanupDL()
				return nil
			}
			fmt.Fprintf(f, "MD5 check failed: want %s, got %s\n"+
				"\tTruncating existing file.\n",
				wantMd5, base64MD5)
			startOffset = 0
		} else if contentLength > 0 && contentLength < startOffset {
			fmt.Fprintf(f, "Server responded with 416 Range Not Satisfiable, but file size does not match existing content.\n"+
				"\tTruncating existing file.\n")
			startOffset = 0
		} else {
			resp.Body.Close()
			resp = nil
			hreq.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
		}
	}

	if resp == nil {
		printRequestHeaders(f, hreq)
		resp, err = client.Do(hreq)
		if resp != nil && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound) {
			cleanupDL()
			return fmt.Errorf("%w: HTTP %d", ErrNoTry, resp.StatusCode)
		}
		if err != nil {
			cleanupDL()
			return fmt.Errorf("HTTP request failed: %w", err)
		}
		defer resp.Body.Close()
		printResponseHeaders(f, resp)
	}

	wantMd5 := TryGetMd5(resp)
	if strings.Contains(urlStr, "tk") && wantMd5 == "" && (resp.ContentLength == 146 || resp.ContentLength == -1) {
		cleanupDL()
		return fmt.Errorf("%w: invalid content length: %d url:%s", ErrNoTry, resp.ContentLength, urlStr)
	}

	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		fmt.Fprintf(f, "Server responded with 416 Range Not Satisfiable, but file size does not match existing content.\n"+
			"\tTruncating existing file.\n")
		startOffset = 0
		resp.Body.Close()
		cleanupDL()
		goto startDownload
	}

	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		cleanupDL()
		return fmt.Errorf("HTTP error: %s", resp.Status)
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text") &&
		(strings.Contains(urlStr, "mp4") || strings.Contains(urlStr, "jpg")) {
		cleanupDL()
		return fmt.Errorf("%w: invalid content type: %s", ErrNoTry, resp.Header.Get("Content-Type"))
	}

	// 处理断点续传的响应
	if startOffset > 0 && resp.StatusCode == 200 && resp.Header.Get("Content-Range") == "" {
		fmt.Fprintf(f, "Server doesn't support resume, restarting download\n")
		slog.Info("Server doesn't support resume, restarting download")
		startOffset = 0
		resp.Body.Close()
		cleanupDL()
		goto startDownload
	}

	// 获取文件总大小用于进度计算
	var totalSize int64
	if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
		parts := strings.Split(contentRange, "/")
		if len(parts) == 2 {
			totalSize, _ = strconv.ParseInt(parts[1], 10, 64)
		}
	} else {
		totalSize = resp.ContentLength
		if startOffset > 0 {
			totalSize += startOffset
		}
	}

	// 打开文件用于写入
	fileFlags := os.O_CREATE | os.O_WRONLY
	if startOffset > 0 {
		fileFlags |= os.O_APPEND
	} else {
		fileFlags |= os.O_TRUNC
	}
	file, err := os.OpenFile(rPath, fileFlags, 0644)
	if err != nil {
		cleanupDL()
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	if startOffset > 0 {
		if _, err = file.Seek(0, io.SeekEnd); err != nil {
			file.Close()
			cleanupDL()
			return fmt.Errorf("failed to seek file: %w", err)
		}
	}

	// 创建进度跟踪器
	var reader io.Reader = resp.Body
	if req.TrackProgress && req.OnProgress != nil && totalSize > 0 {
		lastProgress := 0.0
		lastDownloaded := startOffset
		lastUpdate := time.Now()
		reader = &progressReader{
			reader:     resp.Body,
			total:      totalSize,
			downloaded: startOffset,
			onProgress: func(progress float64, downloaded, total int64) {
				req.OnProgress(progress, downloaded, total)
				minStep := c.progressMinPercentStep
				if minStep <= 0 {
					minStep = 0.5
				}
				maxInterval := c.progressMaxIntervalSeconds
				if maxInterval <= 0 {
					maxInterval = 10
				}
				if f != nil && (progress-lastProgress > minStep || time.Since(lastUpdate) >= time.Duration(maxInterval)*time.Second) {
					bps := float64(downloaded-lastDownloaded) / (time.Since(lastUpdate).Seconds())
					index := 0
					suffixs := []string{"B/s", "KB/s", "MB/s"}
					x := float64(1)
					for bps > 1024 && index < len(suffixs)-1 {
						bps /= 1024
						x *= 1024
						index++
					}
					fmt.Fprintf(f, "%s Progress: %.3f%%  %.2f %s expected time: %.2f s\n", time.Now().Format(time.RFC3339Nano), progress, bps, suffixs[index], (float64(total-downloaded) / bps / x))
					lastProgress = progress
					lastDownloaded = downloaded
					lastUpdate = time.Now()
				}
			},
		}
	}

	written, err := io.Copy(file, reader)
	if err != nil {
		cleanupDL()
		return fmt.Errorf("failed to write file: %w", err)
	}

	if wantMd5 != "" {
		base64MD5, hexMD5, err := computeFileMD5(rPath)
		if err != nil {
			cleanupDL()
			return fmt.Errorf("failed to compute file MD5: %w", err)
		}
		if base64MD5 != wantMd5 && hexMD5 != wantMd5 {
			fmt.Fprintf(f, "MD5 check failed: want %s, got %s (hex: %s)\n"+
				"\tTruncating existing file.\n",
				wantMd5, base64MD5, hexMD5)
			startOffset = 0
			resp.Body.Close()
			cleanupDL()
			goto startDownload
		}
		fmt.Fprintf(f, "MD5 check passed: %s (hex: %s)\n", base64MD5, hexMD5)
		req.Metadata["md5_base64"] = base64MD5
		req.Metadata["md5_hex"] = hexMD5
	}

	modTimeStr := resp.Header.Get("Last-Modified")
	if modTimeStr != "" {
		if modTime, err := time.Parse(time.RFC1123, modTimeStr); err == nil {
			os.Chtimes(rPath, modTime, modTime)
			req.Metadata["mod_time"] = modTime.Format(time.RFC3339Nano)
		}
	}
	if totalSize <= 0 {
		if info, statErr := os.Stat(rPath); statErr == nil && info.Size() > 0 {
			totalSize = info.Size()
		} else {
			totalSize = written
		}
	}
	req.Metadata["total_size"] = strconv.FormatInt(totalSize, 10)
	req.Metadata["status"] = StatusCompleted

	fmt.Fprintf(f, "Download completed, total size: %d bytes\n", totalSize)
	slog.Info("Download completed", "file", req.SavePath, "size", totalSize)
	cleanupDL()
	return nil
}

// ---- FFmpeg HLS 下载处理器 ----

// ffmpegHandler 使用 ffmpeg 下载 HLS (m3u8) 流。
// 注册到全局 handler 注册表中，用于匹配 .m3u8 URL。
type ffmpegHandler struct {
	client *Client
}

func (h *ffmpegHandler) Match(url string) bool {
	return isHlsURL(url)
}

func (h *ffmpegHandler) Name() string {
	return "ffmpeg"
}

func (h *ffmpegHandler) SetClient(c *Client) { h.client = c }

func (h *ffmpegHandler) Download(ctx context.Context, req *Request) error {
	return h.client.downloadHLSWithFFmpeg(ctx, req)
}

// computeFileMD5 计算文件的MD5校验值，返回Base64和十六进制两种格式
func computeFileMD5(filePath string) (string, string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	hasher := md5.New()

	buf := bufferPool.Get().([]byte)
	defer bufferPool.Put(buf)

	if _, err := io.CopyBuffer(hasher, file, buf); err != nil {
		return "", "", err
	}
	hashBytes := hasher.Sum(nil)
	base64MD5 := base64.StdEncoding.EncodeToString(hashBytes)
	hexMD5 := hex.EncodeToString(hashBytes)
	return base64MD5, hexMD5, nil
}
