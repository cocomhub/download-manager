// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dlcore

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
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
)

const (
	StatusPending     = "pending"
	StatusDownloading = "downloading"
	StatusCompleted   = "completed"
	StatusFailed      = "failed"
	StatusCancelled   = "cancelled"
)

var (
	DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
)

// ErrNoTry 表示无需继续重试的错误
var ErrNoTry = errors.New("no try left")

// IsNoTry 判断错误是否属于无需重试类型
func IsNoTry(err error) bool {
	return errors.Is(err, ErrNoTry)
}

type Request struct {
	URL           string
	SavePath      string
	Headers       map[string]string
	TrackProgress bool
	OnProgress    func(progress float64, downloaded, total int64)
	Metadata      map[string]string
}

type Client struct {
	rootDir           string
	logDir            string
	cacheDir          string
	proxies           []string
	forceProxy        bool
	maxRetries        int
	client            *http.Client
	dLimiter          *DomainLimiter
	active            sync.Map
	ffmpegPath        string
	hlsAutoMarkAsFail bool
	// new externalized parameters
	defaultUserAgent                string
	disableInjectBrowserLikeHeaders bool
	proxyDecisionTTLSecs            int
	directProbeTimeoutSecs          int
	bandwidthPathSuffix             string
	progressMinPercentStep          float64
	progressMaxIntervalSeconds      int
	ffmpegExtraArgs                 []string
	moveIfExistsEnabled             bool
	moveIfExistsDir                 string
	externalHLSLogEnabled           bool
	externalHLSLogPath              string
}

func NewClient(opts ...Option) *Client {
	cl := &Client{
		client: &http.Client{
			Timeout: 600 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
		dLimiter: NewDomainLimiter(),
	}
	for _, o := range opts {
		o(cl)
	}
	return cl
}

func (c *Client) ApplyDomainLimits(limits map[string]int) {
	for host, max := range limits {
		c.dLimiter.Set(host, max)
	}
}

func (c *Client) Cancel(url string) error {
	if v, ok := c.active.Load(url); ok {
		cancel := v.(context.CancelFunc)
		cancel()
		c.active.Delete(url)
		return nil
	}
	return fmt.Errorf("no active download for url")
}

func (c *Client) Download(ctx context.Context, req *Request) (err error) {
	if req == nil || req.URL == "" || req.SavePath == "" {
		return fmt.Errorf("invalid request: missing URL or SavePath")
	}

	if strings.Contains(req.URL, ".jpg") || strings.Contains(req.URL, ".jpeg") || strings.Contains(req.URL, ".png") || strings.Contains(req.URL, ".gif") || strings.Contains(req.URL, ".webp") || strings.Contains(req.URL, ".bmp") {
		timeout := time.Second * 30
		if strings.Contains(req.URL, "huaacg.com") {
			timeout = 5 * time.Second
			defer func() {
				if err != nil {
					err = fmt.Errorf("%w: [huaacg] %s", ErrNoTry, err)
					return
				}
			}()
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	rPath := req.SavePath
	if c.rootDir != "" {
		rPath, err = ResolvePath(c.rootDir, req.SavePath)
		if err != nil {
			return err
		}
	}

	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}

	if req.Metadata["status"] == StatusCompleted {
		slog.Info("File already completed, skipping", "file", req.SavePath)
		return nil
	}

	// HLS 场景使用 ffmpeg
	if isHlsURL(req.URL) {
		return c.downloadHLSWithFFmpeg(ctx, req)
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
		logDir := c.logDir
		filename := filepath.Base(rPath)
		if strings.HasPrefix(filename, "0") {
			s := strings.Split(filename, "/")
			if len(s) > 2 {
				filename = s[len(s)-2] + " -- " + s[len(s)-1]
			}
		}
		logFile = filepath.Join(logDir, filename+"."+
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
	if len(c.proxies) > 0 {
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
	defer c.dLimiter.Release(req.URL)
	slog.Info("Starting download", "downloader", "dlcore",
		"url", urlStr, "path", req.SavePath, "log", logFile, "attempt", cnt)

	dctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c.active.Store(req.URL, cancel)

	hreq, err := http.NewRequestWithContext(dctx, "GET", urlStr, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.addBrowserLikeHeaders(req, hreq)

	var resp *http.Response
	// 设置 Range 头支持断点续传
	if startOffset > 0 {
		nreq := hreq.Clone(dctx)
		printRequestHeaders(f, nreq)
		resp, err = client.Do(nreq)
		if resp != nil && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound) {
			return fmt.Errorf("%w: HTTP request failed: %w", ErrNoTry, err)
		}
		if err != nil {
			return fmt.Errorf("HTTP request failed: %w", err)
		}
		defer resp.Body.Close()
		printResponseHeaders(f, resp)

		contentLength, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		if err == nil && (contentLength == startOffset || contentLength == 0 || contentLength == -1 || resp.ContentLength == startOffset) {
			wantMd5 := TryGetMd5(resp)
			if wantMd5 == "" {
				fmt.Fprintf(f, "The file is already fully retrieved; nothing to do.")
				return nil
			}
			base64MD5, hexMD5, err := computeFileMD5(rPath)
			if err != nil {
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
			return fmt.Errorf("%w: HTTP request failed: %w", ErrNoTry, err)
		}
		if err != nil {
			return fmt.Errorf("HTTP request failed: %w", err)
		}
		defer resp.Body.Close()
		printResponseHeaders(f, resp)
	}
	wantMd5 := TryGetMd5(resp)

	if strings.Contains(urlStr, "tk") && (resp.ContentLength == 146 || resp.ContentLength == -1) && wantMd5 == "" {
		return fmt.Errorf("%w: invalid content length: %d url:%s", ErrNoTry, resp.ContentLength, urlStr)
	}

	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		fmt.Fprintf(f, "Server responded with 416 Range Not Satisfiable, but file size does not match existing content.\n"+
			"\tTruncating existing file.\n")
		startOffset = 0
		resp.Body.Close()
		goto startDownload
	}

	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return fmt.Errorf("HTTP error: %s", resp.Status)
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text") &&
		(strings.Contains(urlStr, "mp4") || strings.Contains(urlStr, "jpg")) {
		return fmt.Errorf("%w: invalid content type: %s", ErrNoTry, resp.Header.Get("Content-Type"))
	}

	// 处理断点续传的响应 [10](@ref)
	if startOffset > 0 && resp.StatusCode == 200 && resp.Header.Get("Content-Range") == "" {
		fmt.Fprintf(f, "Server doesn't support resume, restarting download\n")
		slog.Info("Server doesn't support resume, restarting download")
		startOffset = 0 // 服务器不支持断点续传，重新开始下载
		resp.Body.Close()
		goto startDownload
	}

	// 获取文件总大小用于进度计算
	var totalSize int64
	if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
		// 从 Content-Range 头获取总大小，格式: bytes 0-1000/2000
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
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	// 如果续传，定位到文件末尾
	if startOffset > 0 {
		if _, err = file.Seek(0, io.SeekEnd); err != nil {
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
		c.active.Delete(req.URL)
		return fmt.Errorf("failed to write file: %w", err)
	}

	if wantMd5 != "" {
		base64MD5, hexMD5, err := computeFileMD5(rPath)
		if err != nil {
			return fmt.Errorf("failed to compute file MD5: %w", err)
		}
		if base64MD5 != wantMd5 && hexMD5 != wantMd5 {
			fmt.Fprintf(f, "MD5 check failed: want %s, got %s (hex: %s)\n"+
				"\tTruncating existing file.\n",
				wantMd5, base64MD5, hexMD5)
			startOffset = 0
			resp.Body.Close()
			goto startDownload
		}
		fmt.Fprintf(f, "MD5 check passed: %s (hex: %s)\n", base64MD5, hexMD5)
		req.Metadata["md5_base64"] = base64MD5
		req.Metadata["md5_hex"] = hexMD5
	}

	// TODO check
	// if trackProgress && progressObj != nil {
	// 	progressObj.Progress = 100
	// }

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

	// 记录下载完成信息
	fmt.Fprintf(f, "Download completed, total size: %d bytes\n", totalSize)
	slog.Info("Download completed", "file", req.SavePath, "size", totalSize)
	c.active.Delete(req.URL)
	return nil
}

func (c *Client) addBrowserLikeHeaders(req *Request, hreq *http.Request) {
	// 设置请求头模拟浏览器行为
	if !c.disableInjectBrowserLikeHeaders {
		hreq.Header.Set("accept", "*/*")
		hreq.Header.Set("cache-control", "no-cache")
		hreq.Header.Set("pragma", "no-cache")
		hreq.Header.Set("priority", "i")
		hreq.Header.Set("sec-ch-ua", `"Google Chrome";v="143", "Chromium";v="143", "Not A(Brand";v="24"`)
		hreq.Header.Set("sec-ch-ua-mobile", "?0")
		hreq.Header.Set("sec-ch-ua-platform", `"macOS"`)
		hreq.Header.Set("sec-fetch-dest", "video")
		hreq.Header.Set("sec-fetch-mode", "no-cors")
		hreq.Header.Set("sec-fetch-site", "same-origin")
		ua := c.defaultUserAgent
		if strings.TrimSpace(ua) == "" {
			ua = DefaultUserAgent
		}
		if strings.TrimSpace(ua) != "" {
			hreq.Header.Set("user-agent", ua)
		}
	}

	// 添加自定义请求头
	for k, v := range req.Headers {
		hreq.Header.Set(k, v)
	}
}

func TryGetMd5(resp *http.Response) string {
	if resp == nil {
		return ""
	}

	url := resp.Request.URL.String()

	if xAmzMetaMd5 := resp.Header.Get("X-Amz-Meta-Md5chksum"); len(xAmzMetaMd5) == 24 {
		slog.Info("MD5chksum header is not empty", "url", url, "md5", xAmzMetaMd5)
		return xAmzMetaMd5
	} else if etag, ok := strings.CutPrefix(resp.Header.Get("Etag"), "W/"); ok && len(etag) == 34 && etag[0] == '"' && etag[33] == '"' {
		etag = etag[1:33]
		slog.Info("Etag is weak, using it", "url", url, "md5", etag)
		return etag
	} else if wantHexMd5 := resp.Header.Get("Content-MD5"); len(wantHexMd5) == 32 {
		slog.Info("Content-MD5 header is not empty", "url", url, "md5", wantHexMd5)
		return wantHexMd5
	}
	slog.Debug("md5 header is empty", "url", url, "md5", "")
	return ""
}

// progressReader 包装器用于跟踪下载进度
type progressReader struct {
	reader     io.Reader
	total      int64
	downloaded int64
	onProgress func(float64, int64, int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.downloaded += int64(n)
		if pr.total > 0 {
			progress := float64(pr.downloaded) / float64(pr.total) * 100
			pr.onProgress(progress, pr.downloaded, pr.total)
		}
	}
	return n, err
}

type DomainLimiter struct {
	mu    sync.Mutex
	limit map[string]int
	cur   map[string]int
}

func NewDomainLimiter() *DomainLimiter {
	return &DomainLimiter{
		limit: make(map[string]int),
		cur:   make(map[string]int),
	}
}

func (d *DomainLimiter) Set(host string, max int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if max <= 0 {
		max = 1
	}
	d.limit[host] = max
}

func (d *DomainLimiter) Acquire(raw string) {
	u, err := url.Parse(raw)
	if err != nil {
		return
	}
	host := u.Host
	d.mu.Lock()
	max := d.limit[host]
	for max != 0 && d.cur[host] >= max {
		d.mu.Unlock()
		d.mu.Lock()
		max = d.limit[host]
	}
	d.cur[host]++
	d.mu.Unlock()
}

func (d *DomainLimiter) Release(raw string) {
	u, err := url.Parse(raw)
	if err != nil {
		return
	}
	host := u.Host
	d.mu.Lock()
	if d.cur[host] > 0 {
		d.cur[host]--
	}
	d.mu.Unlock()
}

func (c *Client) determineProxy(req *Request) (string, error) {
	targetURL := req.URL
	u, err := url.Parse(targetURL)
	if err != nil {
		return "", err
	}
	domain := u.Host
	cacheBase := c.cacheDir
	if cacheBase == "" {
		cacheBase = filepath.Join(os.TempDir(), "dlcore_proxy_cache")
	}
	if c.rootDir != "" {
		if rp, e := ResolvePath(c.rootDir, cacheBase); e == nil {
			cacheBase = rp
		}
	}
	cachePath := filepath.Join(cacheBase, domain)
	if info, err := os.Stat(cachePath); err == nil {
		slog.Info("cache file exists", "path", cachePath, "mod", info.ModTime())
		ttl := c.proxyDecisionTTLSecs
		if ttl <= 0 {
			ttl = 1
		}
		if time.Since(info.ModTime()) < time.Duration(ttl)*time.Second {
			content, _ := os.ReadFile(cachePath)
			s := strings.TrimSpace(string(content))
			if s == "direct" {
				return "", nil
			}
		}
	}
	if c.checkDirect(req) {
		slog.Info("direct access is available", "url", targetURL)
		_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
		_ = os.WriteFile(cachePath, []byte("direct"), 0644)
		return "", nil
	}
	bestProxy := ""
	minBandwidth := 999999.0
	for _, p := range c.proxies {
		bw := c.getProxyBandwidth(p)
		if bw < minBandwidth {
			minBandwidth = bw
			bestProxy = p
		}
	}
	if bestProxy != "" {
		slog.Info("best proxy found", "proxy", bestProxy, "bandwidth", minBandwidth)
		_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
		_ = os.WriteFile(cachePath, []byte("proxy"), 0644)
		return bestProxy, nil
	}
	return "", fmt.Errorf("no suitable proxy found")
}

func (c *Client) checkDirect(req *Request) bool {
	if c.forceProxy {
		return false
	}
	timeoutSecs := c.directProbeTimeoutSecs
	if timeoutSecs <= 0 {
		timeoutSecs = 3
	}
	client := &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second}
	hreq, err := http.NewRequest("HEAD", req.URL, nil)
	if err != nil {
		return false
	}
	c.addBrowserLikeHeaders(req, hreq)
	resp, err := client.Do(hreq)
	if err != nil || resp.StatusCode != 200 {
		return false
	}
	defer resp.Body.Close()
	return true
}

func (c *Client) getProxyBandwidth(proxyURL string) float64 {
	suffix := c.bandwidthPathSuffix
	if strings.TrimSpace(suffix) == "" {
		suffix = "/bandwidth"
	}
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	target := fmt.Sprintf("%s%s", strings.TrimRight(proxyURL, "/"), suffix)
	timeoutSecs := c.directProbeTimeoutSecs
	if timeoutSecs <= 0 {
		timeoutSecs = 3
	}
	client := &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second}
	resp, err := client.Get(target)
	if err != nil {
		return 999999
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 999999
	}
	val, err := strconv.ParseFloat(strings.TrimSpace(string(body)), 64)
	if err != nil {
		return 999999
	}
	return val
}

func printRequestHeaders(f io.Writer, req *http.Request) {
	fmt.Fprintf(f, "[%s] Request:\n", req.Method)
	fmt.Fprintf(f, "Proto: %s\n", req.Proto)
	fmt.Fprintf(f, "Method: %s\n", req.Method)
	fmt.Fprintf(f, "URL: %s\n", req.URL.String())
	fmt.Fprintf(f, "Headers:\n")
	for k, v := range req.Header {
		fmt.Fprintf(f, "\t%s: %s\n", k, strings.Join(v, ", "))
	}
	fmt.Fprintf(f, "\n")
}

func printResponseHeaders(f io.Writer, resp *http.Response) {
	fmt.Fprintf(f, "[%s] Response:\n", resp.Request.Method)
	fmt.Fprintf(f, "Proto: %s\n", resp.Proto)
	fmt.Fprintf(f, "Status: %s\n", resp.Status)
	fmt.Fprintf(f, "Content-Length: %d\n", resp.ContentLength)
	fmt.Fprintf(f, "Transfer-Encoding: %s\n", resp.TransferEncoding)
	fmt.Fprintf(f, "Connection: %s\n", resp.Header.Get("Connection"))
	fmt.Fprintf(f, "Headers:\n")
	for k, v := range resp.Header {
		fmt.Fprintf(f, "\t%s: %s\n", k, strings.Join(v, ", "))
	}
	fmt.Fprintf(f, "\n")
}

var (
	bufferPool = sync.Pool{
		New: func() any {
			return make([]byte, 64*1024)
		},
	}
)

// computeFileMD5 计算文件的MD5校验值，返回Base64和十六进制两种格式
func computeFileMD5(filePath string) (string, string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	hasher := md5.New()

	// 从池子里获取缓冲区
	buf := bufferPool.Get().([]byte)
	defer bufferPool.Put(buf)

	if _, err := io.CopyBuffer(hasher, file, buf); err != nil {
		return "", "", err
	}
	hashBytes := hasher.Sum(nil)
	// 转换为Base64编码（常见于HTTP头部）
	base64MD5 := base64.StdEncoding.EncodeToString(hashBytes)
	// 转换为十六进制字符串（便于阅读比较）
	hexMD5 := hex.EncodeToString(hashBytes)
	return base64MD5, hexMD5, nil
}
