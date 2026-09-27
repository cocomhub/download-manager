// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/downloader"
	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/scraper_tunnel"
)

var (
	downloadURL  = flag.String("url", "", "URL to download")
	tunnelURL    = flag.String("tunnel", "", "Tunnel URL（默认空，需运行时传入）")
	proxyURL     = flag.String("proxy", "", "Proxy URL（默认空；无前缀=标准代理，gateway:=旧网关式）")
	tunnelSecret = flag.String("tunnel_key", "", "Tunnel key（默认空，需运行时传入）")
	cookie       = flag.String("cookie", "", "Cookie string")
	timeoutSecs  = flag.Int("timeout", 30, "HTTP timeout in seconds")
	verbose      = flag.Bool("v", false, "verbose diagnostics to stderr")
)

// browserHeaders 是抓取请求的浏览器化头（对齐 Chrome 145，Cloudflare 风控放行）。
func browserHeaders(cookie string) map[string]string {
	h := map[string]string{
		"accept":             "*/*",
		"cache-control":      "no-cache",
		"pragma":             "no-cache",
		"priority":           "i",
		"range":              "bytes=0-",
		"sec-ch-ua":          `"Google Chrome";v="145", "Chromium";v="145", "Not A(Brand";v="24"`,
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"macOS"`,
		"sec-fetch-dest":     "video",
		"sec-fetch-mode":     "no-cors",
		"sec-fetch-site":     "same-origin",
		"user-agent":         downloader.DefaultUserAgent,
	}
	if cookie != "" {
		h["cookie"] = cookie
	}
	return h
}

// scraperConfig 是抓取配置（测试可注入）。
type scraperConfig struct {
	proxyURL  string // 代理 URL（可带 gateway: 前缀）
	tunnelURL string
	tunnelKey string
	timeout   int // 秒
	verbose   bool
}

// scraper 是一次抓取的执行器。
type scraper struct {
	cfg     scraperConfig
	client  *http.Client // 直连 client（含超时）
	headers map[string]string
}

// newScraper 创建抓取器。proxyURL 带 gateway: 前缀时生成网关式 client，
// 否则生成标准代理 client（http.Transport.Proxy）。
func newScraper(cfg scraperConfig) *scraper {
	timeout := cfg.timeout
	if timeout <= 0 {
		timeout = 30
	}
	tr := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: time.Duration(timeout) * time.Second,
	}
	kind, rawProxy := download.ParseProxyKind(cfg.proxyURL)
	if rawProxy != "" && kind == download.ProxyKindStandard {
		if pu, err := url.Parse(rawProxy); err == nil {
			tr.Proxy = http.ProxyURL(pu)
		}
	}
	return &scraper{
		cfg:     cfg,
		client:  &http.Client{Transport: tr, Timeout: time.Duration(timeout) * time.Second},
		headers: browserHeaders(""),
	}
}

// fetch 抓取 URL：直连优先，失败后走代理（标准或网关式），tunnel 配置时走隧道。
func (s *scraper) fetch(rawURL string) (string, error) {
	kind, rawProxy := download.ParseProxyKind(s.cfg.proxyURL)

	// 1. 直连（或标准代理 client——newScraper 已设 Proxy）。
	body, err := s.httpGet(rawURL)
	if err == nil {
		return body, nil
	}
	if s.cfg.verbose {
		fmt.Fprintf(os.Stderr, "scraper: direct/standard failed: %v\n", err)
	}

	// 2. 代理 fallback。
	if rawProxy != "" {
		switch kind {
		case download.ProxyKindGateway:
			// 网关式：把目标域名拼进代理 URL 路径（旧行为）。
			u, perr := url.Parse(rawURL)
			if perr == nil {
				p := strings.TrimRight(rawProxy, "/") + "/" + u.Host + u.Path
				if u.RawQuery != "" {
					p += "?" + u.RawQuery
				}
				if s.cfg.verbose {
					fmt.Fprintf(os.Stderr, "scraper: gateway proxy: %s\n", p)
				}
				body, gerr := s.httpGet(p)
				if gerr == nil {
					return body, nil
				}
				return "", gerr
			}
		default:
			// 标准代理已在 client 上；直连失败即代理失败（同一 client）。
			return "", err
		}
	}

	// 3. tunnel 模式（sproxy 加密隧道）。
	if s.cfg.tunnelURL != "" && !strings.Contains(rawURL, "hanime1") {
		header := make(map[string]string)
		for k, v := range s.headers {
			header[k] = v
		}
		body, terr := tunnel.TunnelRequest(&tunnel.SclientConfig{
			ServerURL:        s.cfg.tunnelURL,
			UploadEndpoint:   "/upload",
			DownloadEndpoint: "/download",
			DeleteEndpoint:   "/delete",
			CheckMD5:         false,
			Timeout:          s.cfg.timeout,
			TunnelKey:        s.cfg.tunnelKey,
			TunnelEndpoint:   "/tunnel",
		}, "GET", rawURL, header, "", false, false)
		if terr == nil {
			return body, nil
		}
		return "", terr
	}

	return "", err
}

// httpGet 执行一次 HTTP GET，返回 body。
func (s *scraper) httpGet(rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	for k, v := range s.headers {
		req.Header.Set(k, v)
	}
	// cookie 单独处理（fetch 后设置——headers 已含 cookie）。
	if c := s.headers["cookie"]; c != "" {
		req.Header.Set("cookie", c)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, rawURL)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func main() {
	flag.Parse()

	if *downloadURL == "" && len(flag.Args()) > 0 {
		*downloadURL = flag.Args()[0]
	}
	if *downloadURL == "" {
		log.Fatal("url required")
	}

	cfg := scraperConfig{
		proxyURL:  *proxyURL,
		tunnelURL: *tunnelURL,
		tunnelKey: *tunnelSecret,
		timeout:   *timeoutSecs,
		verbose:   *verbose,
	}
	s := newScraper(cfg)
	s.headers = browserHeaders(*cookie)

	body, err := s.fetch(*downloadURL)
	if err != nil {
		log.Fatal(err)
	}
	_, _ = io.WriteString(os.Stdout, body)
}
