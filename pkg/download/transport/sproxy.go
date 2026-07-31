// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/logutil"
	"github.com/cocomhub/sproxy/pkg/tunnel"
)

// dnsCacheEntry 缓存一个 DNS 解析结果及其过期时间。
type dnsCacheEntry struct {
	addrs    []string
	expireAt time.Time
}

// compile-time interface checks
var _ download.Transport = (*SproxyTunnelTransport)(nil)

// SproxyTunnelTransport 通过 sproxy 传输数据。
// 支持两种模式：
//   - 加密隧道模式（配置 TunnelKey）：使用 tunnel.Client 做 AES-256-GCM 加密隧道
//   - HTTP 代理模式（无 TunnelKey）：通过 sproxy 的 HTTP 代理转发
type SproxyTunnelTransport struct {
	serverURL   string
	client      *http.Client
	tunnelCl    *tunnel.Client
	useTunnel   bool
	logger      *slog.Logger
	dnsCache    sync.Map
	dnsCacheTTL time.Duration
}

// NewSproxyTunnelTransport 创建 SproxyTunnelTransport。
// serverURL 为 sproxy 服务地址，如 "http://localhost:18083"。
func NewSproxyTunnelTransport(serverURL string, opts ...SproxyOption) *SproxyTunnelTransport {
	serverURL = strings.TrimRight(serverURL, "/")
	baseClient := &http.Client{
		Timeout: 600 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        20,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     60 * time.Second,
		},
	}

	t := &SproxyTunnelTransport{
		serverURL:   serverURL,
		client:      baseClient,
		logger:      slog.Default(),
		dnsCacheTTL: 5 * time.Minute,
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// SproxyOption 配置 SproxyTunnelTransport。
type SproxyOption func(*SproxyTunnelTransport)

// WithSproxyTunnelKey 设置隧道密钥（AES-256-GCM，64 hex chars）。
func WithSproxyTunnelKey(key string) SproxyOption {
	return func(t *SproxyTunnelTransport) {
		tc, err := tunnel.NewClient(key, t.serverURL+"/tunnel", 600*time.Second, t.logger)
		if err != nil {
			t.logger.Warn("sproxy: tunnel client init failed, falling back to HTTP proxy", logutil.LogKeyError, err)
			return
		}
		t.tunnelCl = tc
		t.useTunnel = true
	}
}

// WithSproxyHTTPClient 设置自定义 HTTP 客户端。
func WithSproxyHTTPClient(c *http.Client) SproxyOption {
	return func(t *SproxyTunnelTransport) { t.client = c }
}

func (t *SproxyTunnelTransport) Name() string { return "sproxy" }

// TunnelActive 返回当前是否使用加密隧道模式。
func (t *SproxyTunnelTransport) TunnelActive() bool { return t.useTunnel }

// isSafeTargetURL 验证目标 URL 是否安全（防止 SSRF）。
// 拒绝私有 IP、回环地址、link-local 地址和非 http/https scheme。
// DNS 解析结果带 TTL 缓存（5 分钟），避免每次 RoundTrip 重新解析。
func (t *SproxyTunnelTransport) isSafeTargetURL(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid target URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme: %s", parsed.Scheme)
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return fmt.Errorf("empty hostname in target URL")
	}

	// 检查缓存
	if entry, ok := t.dnsCache.Load(hostname); ok {
		cached, ok := entry.(dnsCacheEntry)
		if !ok {
			slog.Warn("sproxy: unexpected type in DNS cache, skipping", "hostname", hostname)
		} else if time.Now().Before(cached.expireAt) {
			for _, addr := range cached.addrs {
				ip := net.ParseIP(addr)
				if ip == nil {
					continue
				}
				if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
					return fmt.Errorf("target resolves to protected address %s", addr)
				}
			}
			return nil
		}
	}

	resolver := net.Resolver{PreferGo: true}
	addrs, err := resolver.LookupHost(ctx, hostname)
	if err != nil {
		return fmt.Errorf("failed to resolve target host: %w", err)
	}
	// 写入缓存
	t.dnsCache.Store(hostname, dnsCacheEntry{
		addrs:    addrs,
		expireAt: time.Now().Add(t.dnsCacheTTL),
	})
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return fmt.Errorf("target resolves to protected address %s", addr)
		}
	}
	return nil
}

// RoundTrip 实现 download.Transport 接口。
func (t *SproxyTunnelTransport) RoundTrip(ctx context.Context, treq *download.TransportRequest) (*download.TransportResponse, error) {
	if t.useTunnel && t.tunnelCl != nil {
		return t.roundTripViaTunnel(ctx, treq)
	}
	return t.roundTripViaProxy(ctx, treq)
}

// roundTripViaProxy 通过 sproxy HTTP 代理转发（非加密）。
func (t *SproxyTunnelTransport) roundTripViaProxy(ctx context.Context, treq *download.TransportRequest) (*download.TransportResponse, error) {
	if err := t.isSafeTargetURL(ctx, treq.URL); err != nil {
		return nil, fmt.Errorf("sproxy: blocked unsafe URL: %w", err)
	}

	targetURL := treq.URL
	targetURL = strings.TrimPrefix(targetURL, "http://")
	targetURL = strings.TrimPrefix(targetURL, "https://")
	fullURL := t.serverURL + "/" + targetURL

	var body io.Reader
	if len(treq.Body) > 0 {
		body = bytes.NewReader(treq.Body)
	}
	hreq, err := http.NewRequestWithContext(ctx, treq.Method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("sproxy: create request: %w", err)
	}
	for k, v := range treq.Headers {
		hreq.Header.Set(k, v)
	}
	if treq.Range != nil && treq.Range.Offset > 0 {
		hreq.Header.Set("Range", fmt.Sprintf("bytes=%d-", treq.Range.Offset))
	}

	resp, err := t.client.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("sproxy: request failed: %w", err)
	}

	headers := make(map[string]string)
	for k := range resp.Header {
		headers[k] = strings.Join(resp.Header.Values(k), ", ")
	}
	return &download.TransportResponse{
		Body:          resp.Body,
		StatusCode:    resp.StatusCode,
		ContentLength: resp.ContentLength,
		Headers:       headers,
		ProxyURL:      t.serverURL,
	}, nil
}

// roundTripViaTunnel 通过 sproxy 加密隧道转发请求。
func (t *SproxyTunnelTransport) roundTripViaTunnel(ctx context.Context, treq *download.TransportRequest) (*download.TransportResponse, error) {
	if err := t.isSafeTargetURL(ctx, treq.URL); err != nil {
		return nil, fmt.Errorf("sproxy: blocked unsafe URL: %w", err)
	}

	var body io.Reader
	if len(treq.Body) > 0 {
		body = bytes.NewReader(treq.Body)
	}
	hreq, err := http.NewRequestWithContext(ctx, treq.Method, treq.URL, body)
	if err != nil {
		return nil, fmt.Errorf("sproxy: tunnel create request: %w", err)
	}
	for k, v := range treq.Headers {
		hreq.Header.Set(k, v)
	}
	if treq.Range != nil && treq.Range.Offset > 0 {
		hreq.Header.Set("Range", fmt.Sprintf("bytes=%d-", treq.Range.Offset))
	}

	resp, err := t.tunnelCl.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("sproxy: tunnel roundtrip: %w", err)
	}

	headers := make(map[string]string)
	for k := range resp.Header {
		headers[k] = strings.Join(resp.Header.Values(k), ", ")
	}
	return &download.TransportResponse{
		Body:          resp.Body,
		StatusCode:    resp.StatusCode,
		ContentLength: resp.ContentLength,
		Headers:       headers,
		ProxyURL:      t.serverURL,
	}, nil
}

// HealthCheck 检查 sproxy 服务是否健康。
func (t *SproxyTunnelTransport) HealthCheck(ctx context.Context) error {
	healthURL := t.serverURL + "/healthz"
	hreq, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return err
	}
	resp, err := t.client.Do(hreq)
	if err != nil {
		return fmt.Errorf("sproxy: health check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sproxy: health check returned %d", resp.StatusCode)
	}
	return nil
}
