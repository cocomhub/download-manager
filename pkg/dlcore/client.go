// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dlcore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	StatusFailedPermanent = "failed_permanent"
	StatusPending         = "pending"
	StatusDownloading     = "downloading"
	StatusCompleted       = "completed"
	StatusFailed          = "failed"
	StatusCancelled       = "cancelled"
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
	defaultHandler    Handler
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
		dLimiter:       NewDomainLimiter(),
		defaultHandler: &httpHandler{},
	}
	for _, o := range opts {
		o(cl)
	}
	if h, ok := cl.defaultHandler.(*httpHandler); ok {
		h.client = cl
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

	// 图片类型 URL 自动设置较短超时，避免长时间挂起
	if isImageURL(req.URL) {
		timeout := 30 * time.Second
		if strings.Contains(req.URL, "huaacg.com") {
			timeout = 5 * time.Second
			defer func() {
				if err != nil {
					err = fmt.Errorf("%w: [huaacg] %v", ErrNoTry, err)
				}
			}()
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}

	if req.Metadata["status"] == StatusCompleted {
		slog.Info("File already completed, skipping", "file", req.SavePath)
		return nil
	}

	// Handler 分发：遍历全局注册表，匹配时使用对应 Handler
	// 无匹配时使用默认 HTTP 下载处理器
	handler := matchHandler(req.URL)
	if handler == nil {
		handler = c.defaultHandler
	}
	return handler.Download(ctx, req)
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
	cond  *sync.Cond
	limit map[string]int
	cur   map[string]int
}

func NewDomainLimiter() *DomainLimiter {
	d := &DomainLimiter{
		limit: make(map[string]int),
		cur:   make(map[string]int),
	}
	d.cond = sync.NewCond(&d.mu)
	return d
}

func (d *DomainLimiter) Set(host string, max int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if max <= 0 {
		max = 1
	}
	d.limit[host] = max
	d.cond.Broadcast()
}

func (d *DomainLimiter) Acquire(raw string) {
	u, err := url.Parse(raw)
	if err != nil {
		return
	}
	host := u.Host
	d.mu.Lock()
	for max := d.limit[host]; max != 0 && d.cur[host] >= max; max = d.limit[host] {
		d.cond.Wait()
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
	d.cond.Broadcast()
	d.mu.Unlock()
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
	return CheckDirect(req.URL, c.forceProxy, timeoutSecs)
}

func (c *Client) getProxyBandwidth(proxyURL string) float64 {
	suffix := c.bandwidthPathSuffix
	timeoutSecs := c.directProbeTimeoutSecs
	return GetProxyBandwidth(proxyURL, suffix, timeoutSecs)
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
