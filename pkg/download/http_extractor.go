// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/pkg/logutil"
)

// mediaExtensionSet 是预期为媒体文件的 URL 扩展名集合，用于 Content-Type 校验。
// 当 URL 扩展名在此集合中但响应 Content-Type 不匹配期望类型时，报 ErrNoTry。
const (
	mimePrefixVideo = "video/"
	mimePrefixImage = "image/"
	chromeVersion   = "145"
)

var mediaExtensionSet = map[string]string{
	".mp4":  mimePrefixVideo,
	".jpg":  mimePrefixImage,
	".jpeg": mimePrefixImage,
	".png":  mimePrefixImage,
	".gif":  mimePrefixImage,
	".webp": mimePrefixImage,
	".bmp":  mimePrefixImage,
	".avif": mimePrefixImage,
	".heic": mimePrefixImage,
	".av1":  mimePrefixVideo,
}

const logTimestampFmt = "20060102150405"

// HTTPExtractor 是通用 HTTP 文件下载编排器。
// 它使用 Transport 做字节传输，自己管理重试、断点续传、MD5 校验。
type HTTPExtractor struct {
	mu             sync.RWMutex
	transport      Transport
	selector       Selector
	maxRetries     int
	rootDir        string
	logDir         string
	ua             string
	allowPaths     []string
	browserHdrs    bool
	cancels        sync.Map // map[string]context.CancelFunc
	responseChecks []ResponseCheck
	// RedactSensitiveHeaders 控制是否在日志中对敏感头脱敏。默认 true。
	redactSensitiveHeaders bool
	// followSymlinks 控制是否解析符号链接。默认 true（安全检查）。
	// 设为 false 时，ResolvePath/IsWithinRoot 不解析符号链接，适用于路径中存在
	// 尚未创建的文件（如下载前）且 rootDir 可能涉及系统符号链接的场景。
	followSymlinks bool
	// TestHookRetrySleep 是测试钩子，在进入重试 sleep 前被调用。仅测试使用。
	TestHookRetrySleep func()
}

// SetFollowSymlinks 设置符号链接解析开关。
// 默认 true（解析符号链接，安全检查）。
// 设为 false 时，ResolvePath/IsWithinRoot 不解析符号链接，适用于路径中存在
// 尚未创建的文件（如下载前）且 rootDir 可能涉及系统符号链接的场景。
func (e *HTTPExtractor) SetFollowSymlinks(v bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.followSymlinks = v
}

// SetBrowserHeaders 控制是否注入 Chrome 风格浏览器标头。
func (e *HTTPExtractor) SetBrowserHeaders(v bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.browserHdrs = v
}

// AddResponseCheck 注册一个响应校验函数，在每次下载拿到响应后执行。
// 多个 check 按注册顺序执行，任一返回 error 则终止下载。
func (e *HTTPExtractor) AddResponseCheck(fn ResponseCheck) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.responseChecks = append(e.responseChecks, fn)
}

// Cancel 实现 Canceller 接口，按 URL 取消正在进行的下载。
// 使用 Load 而非 LoadAndDelete，因为 Extract 的 defer 会清理 e.cancels。
// 即使使用 Load，cancel 闭包在被提取后仍然有效，因为 defer dlCancel() 会处理上下文。
func (e *HTTPExtractor) Cancel(url string) error {
	if v, ok := e.cancels.Load(url); ok {
		if cancel, ok := v.(context.CancelFunc); ok {
			cancel()
		}
	}
	return nil
}

// NewHTTPExtractor 创建并返回 HTTPExtractor 实例。
func NewHTTPExtractor() *HTTPExtractor {
	return NewHTTPExtractorWithConfig(5, "", "", "")
}

// NewHTTPExtractorWithConfig 根据配置创建 HTTPExtractor 实例。
func NewHTTPExtractorWithConfig(maxRetries int, userAgent, rootDir, logDir string) *HTTPExtractor {
	if userAgent == "" {
		userAgent = fmt.Sprintf("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36", chromeVersion)
	}
	return &HTTPExtractor{
		maxRetries:             maxRetries,
		rootDir:                rootDir,
		logDir:                 logDir,
		ua:                     userAgent,
		redactSensitiveHeaders: true,
		followSymlinks:         true,
	}
}

// SetTransport 注入 Transport 实例（实现 ExtractorWithTransport 接口）。
func (e *HTTPExtractor) SetTransport(t Transport) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.transport = t
}

// SetSelector 注入 Selector 实例（实现 ExtractorWithSelector 接口）。
func (e *HTTPExtractor) SetSelector(s Selector) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.selector = s
}

// SetAllowPaths 设置下载路径白名单（可选）。
func (e *HTTPExtractor) SetAllowPaths(paths []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.allowPaths = paths
}

// SetRedactSensitiveHeaders 设置敏感头脱敏开关。
func (e *HTTPExtractor) SetRedactSensitiveHeaders(v bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.redactSensitiveHeaders = v
}

// Name 返回提取器名称。
func (e *HTTPExtractor) Name() string { return "http" }

// Match 判断是否适合处理该 URL。HTTPExtractor 处理非 m3u8 的 URL。
func (e *HTTPExtractor) Match(ctx context.Context, url string) bool {
	return !strings.Contains(strings.ToLower(url), ".m3u8")
}

// Extract 执行完整的 HTTP 文件下载编排。
func (e *HTTPExtractor) Extract(ctx context.Context, req *Request) error {
	// 局部拷贝所有可变字段，避免后续代码在持有锁的情况下长时间运行
	e.mu.RLock()
	localTransport := e.transport
	localSelector := e.selector
	localBrowserHdrs := e.browserHdrs
	localAllowPaths := e.allowPaths
	localMaxRetries := e.maxRetries
	localRootDir := e.rootDir
	localLogDir := e.logDir
	localUA := e.ua
	localFollowSymlinks := e.followSymlinks
	localResponseChecks := make([]ResponseCheck, len(e.responseChecks))
	copy(localResponseChecks, e.responseChecks)
	localRedactSensitiveHeaders := e.redactSensitiveHeaders

	e.mu.RUnlock()

	// 确保 Transport 已注入
	if localTransport == nil {
		return fmt.Errorf("http: transport not set, call SetTransport before Extract")
	}

	// 创建 per-URL 可取消的 context，支持按 URL 精确取消
	dlCtx, dlCancel := context.WithCancel(ctx)
	defer dlCancel()
	// 使用 LoadOrStore 检测相同 URL 的并发下载
	if _, loaded := e.cancels.LoadOrStore(req.URL, dlCancel); loaded {
		dlCancel()
		return fmt.Errorf("%w: %s", ErrAlreadyDownloading, req.URL)
	}

	defer e.cancels.Delete(req.URL)
	rPath, err := e.resolveSavePath(req, localRootDir, localAllowPaths, localFollowSymlinks)
	if err != nil {
		return err
	}

	proxyURL := e.selectProxy(dlCtx, req, localSelector)
	action := e.resolveDownloadAction(rPath, req)

	if action == ActionSkip {
		e.handleSkipResult(rPath, req)
		return nil
	}

	startOffset := e.prepareDownloadOffset(rPath, action)
	ensureRequestFields(req)

	return e.retryDownload(dlCtx, rPath, req.URL, proxyURL, startOffset, req, localTransport, localLogDir, localMaxRetries, localUA, localBrowserHdrs, localResponseChecks, localRedactSensitiveHeaders)
}

// tryDownload 执行单次下载尝试。返回 success=true 表示下载完成，否则返回错误。
func (e *HTTPExtractor) tryDownload(ctx context.Context, rPath, rawURL, proxyURL string, startOffset int64, req *Request, localTransport Transport, localLogDir string, localUA string, localBrowserHdrs bool, localResponseChecks []ResponseCheck, localRedactSensitiveHeaders bool) (success bool, err error) {
	logWriter := createProgressLogWriter(localLogDir, rPath)
	if c, ok := logWriter.(io.Closer); ok {
		defer c.Close()
	}
	started := time.Now()
	defer func() {
		writeDownloadError(logWriter, err)
	}()

	logDownloadStart(logWriter, rPath, proxyURL, rawURL)

	treq := &TransportRequest{
		URL:      rawURL,
		Method:   "GET",
		ProxyURL: proxyURL,
		Headers:  e.buildHeaders(req, localUA, localBrowserHdrs),
	}
	if startOffset > 0 {
		treq.Range = &RangeRequest{Offset: startOffset}
	}

	tresp, tErr := localTransport.RoundTrip(ctx, treq)
	if tErr != nil {
		return false, tErr
	}
	defer tresp.Body.Close()

	logHTTPHeaders(logWriter, treq, tresp, rawURL, proxyURL, localRedactSensitiveHeaders)

	if handled, success, err := handleHTTPResponseStatus(tresp, logWriter, req, rPath); handled {
		return success, err
	}

	if err := validateContentTypeByExtension(rawURL, tresp, logWriter); err != nil {
		return false, err
	}

	for _, check := range localResponseChecks {
		if err := check(req, tresp); err != nil {
			writeLog(logWriter, "Response check failed: %v\n", err)
			return false, err
		}
	}

	if startOffset > 0 && tresp.StatusCode == http.StatusOK {
		writeLog(logWriter, "Server doesn't support resume, restarting download\n")
		return false, nil
	}

	totalSize := getContentLength(tresp)
	if startOffset > 0 && totalSize > 0 && totalSize < startOffset {
		slog.Info("Server content changed during resume, restarting download", "file", rPath, "serverSize", totalSize, "localSize", startOffset)
		return false, nil
	}

	if err := writeResponseBody(tresp.Body, rPath, startOffset, totalSize, req, logWriter); err != nil {
		return false, err
	}

	req.Result.StatusCode = tresp.StatusCode
	req.Result.ContentLength = tresp.ContentLength
	req.Result.TotalSize = totalSize

	if restart, err := checkFileMD5(tresp, rPath, req, logWriter); err != nil {
		return false, err
	} else if restart {
		return false, nil
	}

	saveETagAndModTime(tresp, req, rPath)
	logDownloadComplete(logWriter, started, totalSize)

	return true, nil
}

// ---------------------------------------------------------------------------
// 以下为 tryDownload 提取出的辅助函数
// ---------------------------------------------------------------------------

// createProgressLogWriter 创建进度日志文件。logDir 为空时返回 nil。
func createProgressLogWriter(logDir, rPath string) io.Writer {
	if logDir == "" {
		return nil
	}
	logFileName := filepath.Base(rPath)
	logFile := filepath.Join(logDir, logFileName+"."+
		time.Now().Format(logTimestampFmt)+".progress.log")
	f, fErr := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if fErr != nil {
		slog.Warn("Failed to create progress log file", "file", logFile, logutil.LogKeyError, fErr)
		return nil
	}
	return f
}

// writeDownloadError 将失败信息写入日志（用在 defer 中）。
func writeDownloadError(w io.Writer, err error) {
	if err != nil && w != nil {
		fmt.Fprintf(w, "%s Download failed: %v\n",
			time.Now().Format(time.RFC3339Nano), err)
	}
}

// writeLog 条件写日志，w 为 nil 时静默跳过。
func writeLog(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format, args...)
}

// redactProxyURL 脱敏代理 URL 中的用户名密码。
func redactProxyURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	parsed.User = nil
	return parsed.String()
}

// logDownloadStart 在进度日志开头写入保存路径、代理、URL 信息。
func logDownloadStart(w io.Writer, rPath, proxyURL, rawURL string) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "Save file to %s\n", rPath)
	if proxyURL != "" {
		fmt.Fprintf(w, "Using proxy: %s\n", redactProxyURL(proxyURL))
	} else {
		fmt.Fprintf(w, "Direct connection\n")
	}
	fmt.Fprintf(w, "Requesting URL: %s\n\n", rawURL)
}

// IsSensitiveHeader 判断 HTTP 头名是否为敏感头。导出供子包复用。
func IsSensitiveHeader(name string) bool {
	lower := strings.ToLower(name)
	sensitivePrefixes := []string{
		"authorization", "cookie", "proxy-authorization",
		"x-api-key", "x-auth", "token", "secret", "auth",
	}
	for _, prefix := range sensitivePrefixes {
		if lower == prefix || strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// logHTTPHeaders 在进度日志中输出请求和响应的 HTTP 头（敏感头脱敏）。
func logHTTPHeaders(w io.Writer, treq *TransportRequest, tresp *TransportResponse, rawURL, proxyURL string, redactSensitive bool) {
	if w == nil {
		return
	}

	fmt.Fprintf(w, "[%s] Request:\n", treq.Method)
	fmt.Fprintf(w, "URL: %s\n", rawURL)
	if treq.ProxyURL != "" {
		fmt.Fprintf(w, "Proxy: %s\n", redactProxyURL(treq.ProxyURL))
	} else if proxyURL != "" {
		fmt.Fprintf(w, "Proxy: %s\n", redactProxyURL(proxyURL))
	}
	fmt.Fprintf(w, "Headers:\n")
	for k, v := range treq.Headers {
		if redactSensitive && IsSensitiveHeader(k) {
			v = "[REDACTED]"
		}
		fmt.Fprintf(w, "\t%s: %s\n", k, v)
	}
	if treq.Range != nil && treq.Range.Offset > 0 {
		fmt.Fprintf(w, "\tRange: bytes=%d-\n", treq.Range.Offset)
	}
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "[%d] Response:\n", tresp.StatusCode)
	if statusText := http.StatusText(tresp.StatusCode); statusText != "" {
		fmt.Fprintf(w, "Status: %d %s\n", tresp.StatusCode, statusText)
	}
	fmt.Fprintf(w, "Content-Length: %d\n", tresp.ContentLength)
	fmt.Fprintf(w, "Headers:\n")
	for k, v := range tresp.Headers {
		if redactSensitive && IsSensitiveHeader(k) {
			v = "[REDACTED]"
		}
		fmt.Fprintf(w, "\t%s: %s\n", k, v)
	}
	fmt.Fprintf(w, "\n")
}

// handleHTTPResponseStatus 根据 HTTP 状态码决定后续流程。
// 返回值 handled=true 时，调用方应立即返回 (success, err)。
func handleHTTPResponseStatus(tresp *TransportResponse, w io.Writer, req *Request, rPath string) (handled, success bool, err error) {
	switch {
	case tresp.StatusCode == http.StatusForbidden || tresp.StatusCode == http.StatusNotFound:
		return true, false, fmt.Errorf("%w: HTTP %d", ErrNoTry, tresp.StatusCode)
	case tresp.StatusCode == http.StatusNotModified:
		handle304Response(tresp, w, req, rPath)
		return true, true, nil
	case tresp.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		writeLog(w, "Server responded with 416 Range Not Satisfiable, restarting download\n")
		return true, false, nil
	case tresp.StatusCode != http.StatusOK && tresp.StatusCode != http.StatusPartialContent:
		if tresp.StatusCode >= 400 {
			return true, false, fmt.Errorf("HTTP %d", tresp.StatusCode)
		}
		return true, false, fmt.Errorf("HTTP error: %d", tresp.StatusCode)
	default:
		return false, false, nil
	}
}

// handle304Response 处理 304 Not Modified 响应，填充 req.Result 并保存 ETag。
func handle304Response(tresp *TransportResponse, w io.Writer, req *Request, rPath string) {
	writeLog(w, "Server responded with 304 Not Modified, file unchanged\n")
	req.Result.StatusCode = http.StatusNotModified
	req.Result.ContentLength = 0
	req.Result.TotalSize = 0
	if fi, stErr := os.Stat(rPath); stErr == nil {
		req.Result.TotalSize = fi.Size()
		req.Result.ContentLength = fi.Size()
	} else {
		slog.Warn("Failed to stat file for 304 handling", "file", rPath, logutil.LogKeyError, stErr)
	}
	if req.OnProgress != nil {
		req.OnProgress(100, req.Result.TotalSize, req.Result.TotalSize)
	}
	saveEtag(tresp, req)
}

// saveEtag 将响应的 ETag 保存到 req.Metadata。
func saveEtag(tresp *TransportResponse, req *Request) {
	if etag := tresp.Headers["Etag"]; etag != "" {
		setReqMetadata(req, "etag", etag)
	} else if etag := tresp.Headers["ETag"]; etag != "" {
		setReqMetadata(req, "etag", etag)
	}
}

// validateContentTypeByExtension 检查 URL 媒体扩展名与响应 Content-Type 是否匹配。
func validateContentTypeByExtension(rawURL string, tresp *TransportResponse, w io.Writer) error {
	mediaExt := ""
	if parsedURL, parseErr := url.Parse(rawURL); parseErr == nil {
		mediaExt = strings.ToLower(filepath.Ext(parsedURL.Path))
	} else {
		slog.Warn("Failed to parse URL for content-type validation", logutil.LogKeyURL, rawURL, logutil.LogKeyError, parseErr)
	}
	if ext := mediaExt; mediaExtensionSet[ext] != "" {
		expectedPrefix := mediaExtensionSet[ext]
		ct := tresp.Headers["Content-Type"]
		if ct == "" {
			slog.Warn("Content-Type header is empty, skipping validation", "ext", ext, logutil.LogKeyURL, rawURL)
			return nil
		}
		if !strings.HasPrefix(ct, expectedPrefix) {
			writeLog(w, "Content-Type mismatch for %s: expected %s*, got %s\n", ext, expectedPrefix, ct)
			return fmt.Errorf("%w: invalid content type: expected %s*, got %s", ErrNoTry, expectedPrefix, ct)
		}
	}
	return nil
}

// getContentLength 从响应中提取完整资源大小（优先使用 Content-Range 中的总长）。
func getContentLength(tresp *TransportResponse) int64 {
	if cr := tresp.Headers["Content-Range"]; cr != "" {
		parts := strings.Split(cr, "/")
		if len(parts) == 2 {
			if parsed, pErr := strconv.ParseInt(parts[1], 10, 64); pErr == nil {
				return parsed
			}
		}
	}
	return tresp.ContentLength
}

// writeResponseBody 将响应体写入文件并通过 ProgressReader 跟踪进度。
func writeResponseBody(body io.ReadCloser, rPath string, startOffset, totalSize int64, req *Request, w io.Writer) error {
	fileFlags := os.O_CREATE | os.O_WRONLY
	if startOffset > 0 {
		fileFlags |= os.O_APPEND
	} else {
		fileFlags |= os.O_TRUNC
	}

	file, fErr := os.OpenFile(rPath, fileFlags, 0644)
	if fErr != nil {
		return fmt.Errorf("failed to open file: %w", fErr)
	}
	defer file.Close()

	if w != nil {
		if startOffset > 0 {
			fmt.Fprintf(w, "File mode: append (resume from offset %d)\n", startOffset)
		} else {
			fmt.Fprintf(w, "File mode: truncate (new download)\n")
		}
		fmt.Fprintf(w, "Expected total: %d bytes\n\n", totalSize)
	}

	reader := buildProgressReader(body, startOffset, totalSize, req, w)
	if _, cErr := io.Copy(file, reader); cErr != nil {
		return fmt.Errorf("failed to write file: %w", cErr)
	}
	// Call Done to ensure 100% progress is reported
	if pr, ok := reader.(*ProgressReader); ok {
		pr.Done()
	}
	return nil
}

// buildProgressReader 创建可选的进度跟踪 reader。条件不满足时返回原 body。
// startOffset 为断点续传时的已下载字节数，用于正确计算进度百分比。
func buildProgressReader(body io.Reader, startOffset, totalSize int64, req *Request, w io.Writer) io.Reader {
	if !req.TrackProgress || req.OnProgress == nil || totalSize <= 0 {
		return body
	}
	onProgress := req.OnProgress
	if w != nil {
		onProgress = ComposeProgress(
			req.OnProgress,
			NewProgressLogCallback(
				WithLogWriter(w),
				WithMinPercentStep(0.5),
				WithMaxInterval(10*time.Second),
			),
		)
	}
	return NewProgressReader(body, startOffset, totalSize, onProgress)
}

// checkFileMD5 验证下载文件的 MD5 校验和。返回 restart=true 表示需要重新下载。
func checkFileMD5(tresp *TransportResponse, rPath string, req *Request, w io.Writer) (restart bool, err error) {
	wantMd5 := TryGetMd5(tresp.Headers)
	if wantMd5 == "" {
		return false, nil
	}

	base64MD5, hexMD5, md5Err := ComputeFileMD5(rPath)
	if md5Err != nil {
		return false, fmt.Errorf("failed to compute MD5: %w", md5Err)
	}

	if base64MD5 != wantMd5 && hexMD5 != wantMd5 {
		slog.Warn("MD5 mismatch, retrying download", "want", wantMd5, "got", base64MD5)
		writeLog(w, "MD5 check failed: want %s, got %s (hex: %s)\n", wantMd5, base64MD5, hexMD5)
		return true, nil
	}

	req.Result.MD5Base64 = base64MD5
	req.Result.MD5Hex = hexMD5
	setReqMetadata(req, "checksum", hexMD5)
	writeLog(w, "MD5 check passed: %s (hex: %s)\n", base64MD5, hexMD5)
	return false, nil
}

// saveETagAndModTime 保存响应中的 ETag 和 Last-Modified 到 req.Result 和 metadata。
func saveETagAndModTime(tresp *TransportResponse, req *Request, rPath string) {
	saveEtag(tresp, req)

	if req.Metadata["etag"] == "" && req.Result.MD5Hex != "" {
		setReqMetadata(req, "etag", `"`+req.Result.MD5Hex+`"`)
	}

	if modTimeStr := tresp.Headers["Last-Modified"]; modTimeStr != "" {
		if modTime, pErr := time.Parse(time.RFC1123, modTimeStr); pErr == nil {
			req.Result.ModTime = modTime.Format(time.RFC3339Nano)
			if err := os.Chtimes(rPath, modTime, modTime); err != nil {
				slog.Warn("Failed to set file modification time", "file", rPath, logutil.LogKeyError, err)
			}
		}
	}
}

// logDownloadComplete 在进度日志中输出下载完成信息与平均速度。
func logDownloadComplete(w io.Writer, started time.Time, totalSize int64) {
	if w == nil {
		return
	}
	elapsed := time.Since(started)
	avgSpeed := float64(totalSize) / elapsed.Seconds()
	speedVal := avgSpeed
	var speedUnit string
	switch {
	case speedVal >= 1<<30:
		speedVal /= 1 << 30
		speedUnit = "GiB/s"
	case speedVal >= 1<<20:
		speedVal /= 1 << 20
		speedUnit = "MiB/s"
	case speedVal >= 1<<10:
		speedVal /= 1 << 10
		speedUnit = "KiB/s"
	default:
		speedUnit = "B/s"
	}
	fmt.Fprintf(w, "Download completed, total size: %d bytes\n", totalSize)
	fmt.Fprintf(w, "Elapsed: %.2f s, average speed: %.2f %s\n",
		elapsed.Seconds(), speedVal, speedUnit)
}

// setReqMetadata 写入 req.Metadata 并触发 OnMetadata 回调（如有），
// 确保调用方能立即持久化，避免 crash 窗口导致 ETag/checksum 丢失。
func setReqMetadata(req *Request, key, value string) {
	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	req.Metadata[key] = value
	if req.OnMetadata != nil {
		req.OnMetadata(key, value)
	}
}

// ---------------------------------------------------------------------------
// 以下为 Extract 提取出的辅助函数
// ---------------------------------------------------------------------------

// resolveSavePath 解析保存路径并创建目录。
func (e *HTTPExtractor) resolveSavePath(req *Request, rootDir string, allowPaths []string, followSymlinks bool) (string, error) {
	rPath := req.SavePath
	if rootDir != "" {
		var err error
		rPath, err = ResolvePathWithAllowList(rootDir, allowPaths, req.SavePath, followSymlinks)
		if err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(rPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	return rPath, nil
}

// selectProxy 选择代理，失败时降级为直连。
func (e *HTTPExtractor) selectProxy(ctx context.Context, req *Request, selector Selector) string {
	if selector == nil {
		return ""
	}
	proxyURL, err := selector.SelectProxy(ctx, req.URL, req.Hint)
	if err != nil {
		slog.Warn("Proxy selection failed, falling back to direct", logutil.LogKeyURL, req.URL, logutil.LogKeyError, err)
		return ""
	}
	return proxyURL
}

// resolveDownloadAction 读取元数据中的 ETag/checksum 并决定下载策略。
func (e *HTTPExtractor) resolveDownloadAction(rPath string, req *Request) DownloadAction {
	prevETag := ""
	prevChecksum := ""
	if req.Metadata != nil {
		prevETag = req.Metadata["etag"]
		prevChecksum = req.Metadata["checksum"]
	}
	return ResolveAction(rPath, prevETag, prevChecksum, os.Stat, func(path string) (string, error) {
		_, hexMD5, err := ComputeFileMD5(path)
		if err == nil {
			// 将计算出的 MD5 写入 req.Result，避免 handleSkipResult 重复计算
			if req.Result == nil {
				req.Result = &DownloadResult{}
			}
			req.Result.MD5Hex = hexMD5
		}
		return hexMD5, err
	})
}

// handleSkipResult 处理文件未变更（ActionSkip）的情况。
func (e *HTTPExtractor) handleSkipResult(rPath string, req *Request) {
	slog.Info("Skipping download — file unchanged (ETag + checksum match)", "file", req.SavePath)
	if req.Result == nil {
		req.Result = &DownloadResult{}
	}
	req.Result.StatusCode = http.StatusNotModified
	fi, err := os.Stat(rPath)
	if err == nil {
		req.Result.ContentLength = fi.Size()
		req.Result.TotalSize = fi.Size()
		req.Result.ModTime = fi.ModTime().Format(time.RFC3339Nano)
	} else {
		req.Result.ContentLength = 0
		req.Result.TotalSize = 0
		slog.Warn("File not found or unreadable after skip result", "file", rPath)
	}
	if req.OnProgress != nil {
		req.OnProgress(100, req.Result.TotalSize, req.Result.TotalSize)
	}
}

// prepareDownloadOffset 检查已有文件并决定起始偏移量，必要时清除失效文件。
func (e *HTTPExtractor) prepareDownloadOffset(rPath string, action DownloadAction) int64 {
	fi, statErr := os.Stat(rPath)
	if statErr != nil || fi.Size() == 0 {
		return 0
	}
	switch action {
	case ActionResume, ActionDownload:
		slog.Info("Resuming download (best-effort)", "file", rPath, "offset", fi.Size())
		return fi.Size()
	case ActionReDownload:
		if err := os.Remove(rPath); err != nil {
			slog.Warn("Failed to remove stale file for re-download", "file", rPath, logutil.LogKeyError, err)
		}
		slog.Info("Removing stale file for re-download", "file", rPath)
	}
	return 0
}

// ensureRequestFields 确保 req.Metadata 和 req.Result 非 nil。
func ensureRequestFields(req *Request) {
	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	if req.Result == nil {
		req.Result = &DownloadResult{}
	}
}

// retryDownload 执行带重试的下载循环。
func (e *HTTPExtractor) retryDownload(dlCtx context.Context, rPath, rawURL, proxyURL string, startOffset int64, req *Request, localTransport Transport, localLogDir string, localMaxRetries int, localUA string, localBrowserHdrs bool, localResponseChecks []ResponseCheck, localRedactSensitiveHeaders bool) error {
	maxRetries := localMaxRetries
	// 将 maxRetries=0 视为"不重试"。
	// 注意：dlcore 将 maxRetries=0 视为"无限重试"，这是两者的行为差异。
	if maxRetries < 0 {
		maxRetries = 5
	}
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-dlCtx.Done():
			return dlCtx.Err()
		default:
		}

		success, err := e.tryDownload(dlCtx, rPath, rawURL, proxyURL, startOffset, req, localTransport, localLogDir, localUA, localBrowserHdrs, localResponseChecks, localRedactSensitiveHeaders)
		if err != nil {
			if IsNoTry(err) {
				return err
			}
			slog.Warn("Download attempt failed, retrying", "attempt", attempt, logutil.LogKeyURL, rawURL, logutil.LogKeyError, err)
			// 测试钩子：在进入 sleep 前通知调用方
			if hook := e.TestHookRetrySleep; hook != nil {
				hook()
			}
			select {
			case <-dlCtx.Done():
				return dlCtx.Err()
			default:
				// 使用 timer 在 ctx 取消时及时释放
				timer := time.NewTimer(time.Duration(attempt) * time.Second)
				select {
				case <-timer.C:
				case <-dlCtx.Done():
					timer.Stop()
					return dlCtx.Err()
				}
			}
			continue
		}
		if !success {
			startOffset = 0
			continue
		}
		return nil
	}
	return fmt.Errorf("%w: max retries reached (%d)", ErrNoTry, maxRetries)
}
func (e *HTTPExtractor) buildHeaders(req *Request, localUA string, localBrowserHdrs bool) map[string]string {
	h := make(map[string]string)
	if req.Headers != nil {
		for k, v := range req.Headers {
			if v != "" {
				h[k] = v
			}
		}
	}
	if _, ok := h["User-Agent"]; !ok && localUA != "" {
		h["User-Agent"] = localUA
	}

	// 注入 Chrome 风格浏览器标头（除非禁用），然后在最后用 req.Headers 覆盖
	if localBrowserHdrs {
		browser := map[string]string{
			"Accept":             "*/*",
			"Cache-Control":      "no-cache",
			"Pragma":             "no-cache",
			"Priority":           "i",
			"Sec-Ch-Ua":          fmt.Sprintf(`"Google Chrome";v=%q, "Chromium";v=%q, "Not A(Brand";v="24"`, chromeVersion, chromeVersion),
			"Sec-Ch-Ua-Mobile":   "?0",
			"Sec-Ch-Ua-Platform": `"macOS"`,
			"Sec-Fetch-Dest":     "video",
			"Sec-Fetch-Mode":     "no-cors",
			"Sec-Fetch-Site":     "cross-site",
		}
		for k, v := range browser {
			if _, exists := h[k]; !exists {
				h[k] = v
			}
		}
	}

	// 如果之前有 ETag 记录，设置 If-None-Match 条件请求头
	if req.Metadata != nil {
		if etag := req.Metadata["etag"]; etag != "" {
			if _, has := h["If-None-Match"]; !has {
				h["If-None-Match"] = etag
			}
		}
	}
	return h
}
