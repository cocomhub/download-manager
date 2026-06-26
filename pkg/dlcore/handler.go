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

	"github.com/cocomhub/download-manager/pkg/logutil"
)

const truncateLogMsg = "\tTruncating existing file."

const logTimestampFmt = "20060102150405"

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

// ClientInjecter 表示 Handler 需要在初始化后注入 Client 引用。
// 当 Handler 需要访问 Client 配置时（代理、日志路径等）应实现此接口。
type ClientInjecter interface {
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

// ---- 辅助函数 ----

// resolveSavePath 解析保存路径，若配置了 rootDir 则拼接。
func resolveSavePath(c *Client, req *Request) (string, error) {
	if c.rootDir == "" {
		return req.SavePath, nil
	}
	return ResolvePath(c.rootDir, req.SavePath)
}

// resolveTargetURL 确定最终的下载 URL，包含代理重写。
func resolveTargetURL(c *Client, req *Request) string {
	if c.proxySelector == nil {
		return req.URL
	}
	proxyURL, err := c.determineProxy(req)
	if err != nil {
		slog.Warn("Proxy selection failed, falling back to direct",
			"url", req.URL, logutil.LogKeyError, err)
		return req.URL
	}
	if proxyURL == "" {
		return req.URL
	}
	urlStr := strings.TrimPrefix(req.URL, "http://")
	urlStr = strings.TrimPrefix(urlStr, "https://")
	urlStr = proxyURL + "/" + urlStr
	slog.Info("Using proxy", logutil.LogKeyURL, urlStr, "proxy", proxyURL)
	return urlStr
}

// getResumeOffset 检查文件是否存在以支持断点续传，返回已有文件大小。
func getResumeOffset(rPath string) int64 {
	fileInfo, err := os.Stat(rPath)
	if err == nil && fileInfo.Size() > 0 {
		slog.Info("Resuming download", "file", filepath.Base(rPath), "offset", fileInfo.Size())
		return fileInfo.Size()
	}
	return 0
}

// openLogFile 创建下载日志文件。
// 未配置 logDir 时返回 (空, nil)。
func openLogFile(c *Client, rPath string) (string, *os.File) {
	if c.logDir == "" {
		return "", nil
	}
	logFileName := filepath.Base(rPath)
	if strings.HasPrefix(logFileName, "0") {
		s := strings.Split(rPath, "/")
		if len(s) > 2 {
			logFileName = s[len(s)-2] + " -- " + s[len(s)-1]
		}
	}
	logPath := filepath.Join(c.logDir, logFileName+"."+
		time.Now().Format(logTimestampFmt)+".native.log")
	ff, err := os.Create(logPath)
	if err != nil {
		slog.Warn("Failed to create log file", "file", logPath, logutil.LogKeyError, err)
		return logPath, nil
	}
	return logPath, ff
}

// handleResumeProbe 执行断点续传的探测请求，检查文件是否已完成、服务器是否支持续传。
//
// 返回值：
//   - (resp, false, nil)：resp 非 nil 时可直接用于下载（body 未关闭）；resp 为 nil 时需发起新请求
//     （hreq 可能已设置 Range 头，resumeOffset 可能已重置为 0）
//   - (nil, true, nil)：文件已完整（元数据已设置），调用方应返回成功
//   - (nil, false, err)：致命错误
func handleResumeProbe(c *Client, dctx context.Context, req *Request,
	rPath string, resumeOffset *int64, hreq *http.Request,
	client *http.Client, f io.Writer,
) (*http.Response, bool, error) {
	nreq := hreq.Clone(dctx)
	printRequestHeaders(f, nreq)
	resp, err := client.Do(nreq)
	if err != nil {
		return nil, false, fmt.Errorf("HTTP request failed: %w", err)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, false, fmt.Errorf("%w: HTTP %d", ErrNoTry, resp.StatusCode)
	}
	printResponseHeaders(f, resp)

	contentLength, parseErr := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if parseErr != nil {
		// 无法解析 Content-Length，按续传处理
		resp.Body.Close()
		hreq.Header.Set("Range", fmt.Sprintf("bytes=%d-", *resumeOffset))
		return nil, false, nil
	}

	// 文件恰好与已有内容匹配——可能是完整的
	if contentLength == *resumeOffset || contentLength == 0 || contentLength == -1 || resp.ContentLength == *resumeOffset {
		return handleResumeCompleteState(resp, rPath, resumeOffset, req, f)
	}

	// 服务器上的文件变小了，截断重下
	if contentLength > 0 && contentLength < *resumeOffset {
		fmt.Fprintf(f, "Server responded with 416 Range Not Satisfiable, but file size does not match existing content.\n"+
			truncateLogMsg+"\n")
		*resumeOffset = 0
		return resp, false, nil
	}

	// 服务器支持断点续传：关闭探测响应，设置 Range 头发起新请求
	resp.Body.Close()
	hreq.Header.Set("Range", fmt.Sprintf("bytes=%d-", *resumeOffset))
	return nil, false, nil
}

// handleResumeCompleteState 处理探测响应中 Content-Length 匹配已有文件的情况。
// 返回复用探测响应、文件完成标记、或错误。
func handleResumeCompleteState(resp *http.Response, rPath string,
	resumeOffset *int64, req *Request, f io.Writer,
) (*http.Response, bool, error) {
	wantMd5 := TryGetMd5(resp)
	if wantMd5 == "" {
		fmt.Fprintf(f, "The file is already fully retrieved; nothing to do.")
		return nil, true, nil
	}

	base64MD5, hexMD5, err := computeFileMD5(rPath)
	if err != nil {
		return nil, false, fmt.Errorf("failed to compute file MD5: %w", err)
	}
	if base64MD5 == wantMd5 || hexMD5 == wantMd5 {
		fmt.Fprintf(f, "MD5 check passed: %s (hex: %s)\n", base64MD5, hexMD5)
		req.Metadata["md5_base64"] = base64MD5
		req.Metadata["md5_hex"] = hexMD5
		if modTimeStr := resp.Header.Get("Last-Modified"); modTimeStr != "" {
			if modTime, modErr := time.Parse(time.RFC1123, modTimeStr); modErr == nil {
				os.Chtimes(rPath, modTime, modTime)
				req.Metadata["mod_time"] = modTime.Format(time.RFC3339Nano)
			}
		}
		if cl, clErr := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64); clErr == nil {
			req.Metadata["total_size"] = strconv.FormatInt(cl, 10)
		}
		req.Metadata["status"] = StatusCompleted
		return nil, true, nil
	}

	fmt.Fprintf(f, "MD5 check failed: want %s, got %s\n"+
		truncateLogMsg+"\n", wantMd5, base64MD5)
	*resumeOffset = 0
	return resp, false, nil
}

// validateResponse 检查响应状态码和内容类型是否可接受。
func validateResponse(resp *http.Response, urlStr, wantMd5 string) error {
	if strings.Contains(urlStr, "tk") && wantMd5 == "" && (resp.ContentLength == 146 || resp.ContentLength == -1) {
		return fmt.Errorf("%w: invalid content length: %d url:%s", ErrNoTry, resp.ContentLength, urlStr)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("HTTP error: %s", resp.Status)
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text") &&
		(strings.Contains(urlStr, "mp4") || strings.Contains(urlStr, "jpg")) {
		return fmt.Errorf("%w: invalid content type: %s", ErrNoTry, resp.Header.Get("Content-Type"))
	}
	return nil
}

// calcTotalSize 从响应头计算下载文件总大小。
func calcTotalSize(resp *http.Response, startOffset int64) int64 {
	if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
		parts := strings.Split(contentRange, "/")
		if len(parts) == 2 {
			total, _ := strconv.ParseInt(parts[1], 10, 64)
			return total
		}
		return 0
	}
	total := resp.ContentLength
	if startOffset > 0 {
		total += startOffset
	}
	return total
}

// openOutputFile 以续传或覆盖模式打开目标文件。
func openOutputFile(rPath string, startOffset int64) (*os.File, error) {
	flags := os.O_CREATE | os.O_WRONLY
	if startOffset > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	file, err := os.OpenFile(rPath, flags, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	if startOffset > 0 {
		if _, seekErr := file.Seek(0, io.SeekEnd); seekErr != nil {
			file.Close()
			return nil, fmt.Errorf("failed to seek file: %w", seekErr)
		}
	}
	return file, nil
}

// checkPostDownloadMD5 下载完成后校验文件 MD5，返回是否需要重试。
func checkPostDownloadMD5(rPath, wantMd5 string, req *Request, f io.Writer) (bool, error) {
	base64MD5, hexMD5, err := computeFileMD5(rPath)
	if err != nil {
		return false, fmt.Errorf("failed to compute file MD5: %w", err)
	}
	if base64MD5 != wantMd5 && hexMD5 != wantMd5 {
		fmt.Fprintf(f, "MD5 check failed: want %s, got %s (hex: %s)\n"+
			truncateLogMsg+"\n", wantMd5, base64MD5, hexMD5)
		return true, nil
	}
	fmt.Fprintf(f, "MD5 check passed: %s (hex: %s)\n", base64MD5, hexMD5)
	req.Metadata["md5_base64"] = base64MD5
	req.Metadata["md5_hex"] = hexMD5
	return false, nil
}

// recordDownloadMetadata 在下载成功后写入最终元数据。
func recordDownloadMetadata(req *Request, resp *http.Response, totalSize int64, rPath string, written int64) {
	if modTimeStr := resp.Header.Get("Last-Modified"); modTimeStr != "" {
		if modTime, modErr := time.Parse(time.RFC1123, modTimeStr); modErr == nil {
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
}

// makeDownloadRequest 执行 HTTP GET 请求并检查 403/404。
func makeDownloadRequest(client *http.Client, dctx context.Context, f io.Writer, hreq *http.Request) (*http.Response, error) {
	printRequestHeaders(f, hreq)
	resp, err := client.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, fmt.Errorf("%w: HTTP %d", ErrNoTry, resp.StatusCode)
	}
	printResponseHeaders(f, resp)
	return resp, nil
}

// newProgressReader 创建进度跟踪 reader。
func newProgressReader(c *Client, req *Request, resp *http.Response, totalSize, startOffset int64, f io.Writer) io.Reader {
	if !req.TrackProgress || req.OnProgress == nil || totalSize <= 0 {
		return resp.Body
	}

	minStep := c.progressMinPercentStep
	if minStep <= 0 {
		minStep = 0.5
	}
	maxInterval := c.progressMaxIntervalSeconds
	if maxInterval <= 0 {
		maxInterval = 10
	}

	lastProgress := 0.0
	lastDownloaded := startOffset
	lastUpdate := time.Now()

	return &progressReader{
		reader:     resp.Body,
		total:      totalSize,
		downloaded: startOffset,
		onProgress: func(progress float64, downloaded, total int64) {
			req.OnProgress(progress, downloaded, total)
			if f != nil && (progress-lastProgress > minStep || time.Since(lastUpdate) >= time.Duration(maxInterval)*time.Second) {
				bps := float64(downloaded-lastDownloaded) / (time.Since(lastUpdate).Seconds())
				index := 0
				suffixes := []string{"B/s", "KB/s", "MB/s"}
				x := float64(1)
				for bps > 1024 && index < len(suffixes)-1 {
					bps /= 1024
					x *= 1024
					index++
				}
				fmt.Fprintf(f, "%s Progress: %.3f%%  %.2f %s expected time: %.2f s\n",
					time.Now().Format(time.RFC3339Nano), progress, bps, suffixes[index],
					(float64(total-downloaded) / bps / x))
				lastProgress = progress
				lastDownloaded = downloaded
				lastUpdate = time.Now()
			}
		},
	}
}

// downloadFromResponse 从 HTTP 响应中下载文件、校验并写入元数据。
// 返回 (完成, 需要重试, 错误)。
//
// 注意：调用方负责在 return 前关闭 resp.Body，本函数 defer 关闭。
func downloadFromResponse(c *Client, req *Request, resp *http.Response,
	rPath, urlStr string, resumeOffset *int64, f io.Writer,
) (completed bool, shouldRetry bool, err error) {
	defer resp.Body.Close()

	wantMd5 := TryGetMd5(resp)

	// 416 Range Not Satisfiable
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		fmt.Fprintf(f, "Server responded with 416 Range Not Satisfiable, but file size does not match existing content.\n"+
			truncateLogMsg+"\n")
		*resumeOffset = 0
		return false, true, nil
	}

	// 校验响应
	if valErr := validateResponse(resp, urlStr, wantMd5); valErr != nil {
		return false, false, valErr
	}

	// 服务器不支持断点续传
	if *resumeOffset > 0 && resp.StatusCode == http.StatusOK && resp.Header.Get("Content-Range") == "" {
		fmt.Fprintf(f, "Server doesn't support resume, restarting download\n")
		slog.Info("Server doesn't support resume, restarting download")
		*resumeOffset = 0
		return false, true, nil
	}

	totalSize := calcTotalSize(resp, *resumeOffset)

	file, fileErr := openOutputFile(rPath, *resumeOffset)
	if fileErr != nil {
		return false, false, fileErr
	}

	reader := newProgressReader(c, req, resp, totalSize, *resumeOffset, f)

	written, copyErr := io.Copy(file, reader)
	if copyErr != nil {
		file.Close()
		return false, false, fmt.Errorf("failed to write file: %w", copyErr)
	}
	file.Close()

	// 下载后 MD5 校验
	if wantMd5 != "" {
		retry, md5Err := checkPostDownloadMD5(rPath, wantMd5, req, f)
		if md5Err != nil {
			return false, false, md5Err
		}
		if retry {
			*resumeOffset = 0
			return false, true, nil
		}
	}

	// 最终元数据
	recordDownloadMetadata(req, resp, totalSize, rPath, written)

	fmt.Fprintf(f, "Download completed, total size: %d bytes\n", totalSize)
	slog.Info("Download completed", "file", req.SavePath, "size", totalSize)
	return true, false, nil
}

// doDownloadAttempt 执行一次完整的下载尝试（含续传探测、HTTP 请求、下载、校验）。
func doDownloadAttempt(ctx context.Context, c *Client, req *Request, urlStr string,
	client *http.Client, rPath string, resumeOffset *int64,
	f io.Writer, attempt int,
) (completed bool, shouldRetry bool, err error) {
	fmt.Fprintf(f, "Requesting URL: %s (Attempt %d)\n\n", urlStr, attempt)
	c.dLimiter.Acquire(req.URL)
	slog.Info("Starting download", "downloader", "dlcore",
		"url", urlStr, "path", req.SavePath, "attempt", attempt)

	dctx, cancel := context.WithCancel(ctx)
	c.active.Store(req.URL, cancel)
	defer func() {
		c.active.Delete(req.URL)
		cancel()
		c.dLimiter.Release(req.URL)
	}()

	hreq, reqErr := http.NewRequestWithContext(dctx, "GET", urlStr, nil)
	if reqErr != nil {
		return false, false, fmt.Errorf("failed to create request: %w", reqErr)
	}
	c.addBrowserLikeHeaders(req, hreq)

	// 续传探测
	if *resumeOffset > 0 {
		probeResp, fileComplete, probeErr := handleResumeProbe(c, dctx, req, rPath, resumeOffset, hreq, client, f)
		if probeErr != nil {
			return false, false, probeErr
		}
		if fileComplete {
			return true, false, nil
		}
		if probeResp != nil {
			return downloadFromResponse(c, req, probeResp, rPath, urlStr, resumeOffset, f)
		}
		// probeResp == nil：hreq 可能已设 Range 头，继续发起真实请求
	}

	resp, reqErr := makeDownloadRequest(client, dctx, f, hreq)
	if reqErr != nil {
		return false, false, reqErr
	}

	return downloadFromResponse(c, req, resp, rPath, urlStr, resumeOffset, f)
}

func (h *httpHandler) Download(ctx context.Context, req *Request) error {
	c := h.client
	var err error

	rPath, err := resolveSavePath(c, req)
	if err != nil {
		return err
	}

	if dirErr := os.MkdirAll(filepath.Dir(rPath), 0755); dirErr != nil {
		return fmt.Errorf("failed to create directory: %w", dirErr)
	}

	logPath, logFileObj := openLogFile(c, rPath)
	var f io.Writer
	f = io.Discard
	if logFileObj != nil {
		defer logFileObj.Close()
		f = logFileObj
	}
	_ = logPath // log file path is only used implicitly via f

	defer func() {
		if err != nil {
			fmt.Fprintf(f, "%s Download failed: %v\n",
				time.Now().Format(time.RFC3339Nano), err)
		}
	}()
	fmt.Fprintf(f, "\n\nSave file to %s\n\n", rPath)

	urlStr := resolveTargetURL(c, req)
	client := c.client
	resumeOffset := getResumeOffset(rPath)

	for attempt := 1; ; attempt++ {
		if c.maxRetries != 0 && attempt > c.maxRetries {
			fmt.Fprintf(f, "Max retries reached: %d\n", c.maxRetries)
			return fmt.Errorf("max retries reached: %d", c.maxRetries)
		}

		completed, retry, dlErr := doDownloadAttempt(
			ctx, c, req, urlStr, client, rPath, &resumeOffset, f, attempt)
		if dlErr != nil {
			err = dlErr
			return dlErr
		}
		if completed {
			return nil
		}
		if !retry {
			return fmt.Errorf("unexpected state: neither completed nor retryable")
		}
	}
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
