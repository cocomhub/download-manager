// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/sproxy/pkg/tunnel"
)

// compile-time interface checks
var _ download.Transport = (*SproxyTunnelTransport)(nil)

// SproxyTunnelTransport 閫氳繃 sproxy 浼犺緭鏁版嵁銆?// 鏀寔涓ょ妯″紡锛?//   - 鍔犲瘑闅ч亾妯″紡锛堥厤缃?TunnelKey锛夛細浣跨敤 tunnel.Client 鍋?AES-256-GCM 鍔犲瘑闅ч亾
//   - HTTP 浠ｇ悊妯″紡锛堟棤 TunnelKey锛夛細閫氳繃 sproxy 鐨?HTTP 浠ｇ悊杞彂
type SproxyTunnelTransport struct {
	serverURL string
	client    *http.Client
	tunnelCl  *tunnel.Client
	useTunnel bool
	logger    *slog.Logger
}

// NewSproxyTunnelTransport 鍒涘缓 SproxyTunnelTransport銆?// serverURL 涓?sproxy 鏈嶅姟鍦板潃锛屽 "http://localhost:18083"銆?func NewSproxyTunnelTransport(serverURL string, opts ...SproxyOption) *SproxyTunnelTransport {
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
		serverURL: serverURL,
		client:    baseClient,
		logger:    slog.Default(),
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// SproxyOption 閰嶇疆 SproxyTunnelTransport銆?type SproxyOption func(*SproxyTunnelTransport)

// WithSproxyTunnelKey 璁剧疆闅ч亾瀵嗛挜锛圓ES-256-GCM锛?4 hex chars锛夈€?func WithSproxyTunnelKey(key string) SproxyOption {
	return func(t *SproxyTunnelTransport) {
		tc, err := tunnel.NewClient(key, t.serverURL+"/tunnel", 600*time.Second, t.logger)
		if err != nil {
			t.logger.Warn("sproxy: tunnel client init failed, falling back to HTTP proxy", "error", err)
			return
		}
		t.tunnelCl = tc
		t.useTunnel = true
	}
}

// WithSproxyHTTPClient 璁剧疆鑷畾涔?HTTP 瀹㈡埛绔€?func WithSproxyHTTPClient(c *http.Client) SproxyOption {
	return func(t *SproxyTunnelTransport) { t.client = c }
}

func (t *SproxyTunnelTransport) Name() string { return "sproxy" }

// isSafeTargetURL 楠岃瘉鐩爣 URL 鏄惁瀹夊叏锛堥槻姝?SSRF锛夈€?// 鎷掔粷绉佹湁 IP銆佸洖鐜湴鍧€銆乴ink-local 鍦板潃鍜岄潪 http/https scheme銆?func isSafeTargetURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid target URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme: %s", parsed.Scheme)
	}
	addrs, err := net.LookupHost(parsed.Hostname())
	if err != nil {
		return fmt.Errorf("failed to resolve target host: %w", err)
	}
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

// RoundTrip 瀹炵幇 download.Transport 鎺ュ彛銆?func (t *SproxyTunnelTransport) RoundTrip(ctx context.Context, treq *download.TransportRequest) (*download.TransportResponse, error) {
	if t.useTunnel && t.tunnelCl != nil {
		return t.roundTripViaTunnel(ctx, treq)
	}
	return t.roundTripViaProxy(ctx, treq)
}

// roundTripViaProxy 閫氳繃 sproxy HTTP 浠ｇ悊杞彂锛堥潪鍔犲瘑锛夈€?func (t *SproxyTunnelTransport) roundTripViaProxy(ctx context.Context, treq *download.TransportRequest) (*download.TransportResponse, error) {
	if err := isSafeTargetURL(treq.URL); err != nil {
		return nil, fmt.Errorf("sproxy: blocked unsafe URL: %w", err)
	}

	targetURL := treq.URL
	targetURL = strings.TrimPrefix(targetURL, "http://")
	targetURL = strings.TrimPrefix(targetURL, "https://")
	fullURL := t.serverURL + "/" + targetURL

	hreq, err := http.NewRequestWithContext(ctx, treq.Method, fullURL, nil)
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
		headers[k] = resp.Header.Get(k)
	}
	return &download.TransportResponse{
		Body:          resp.Body,
		StatusCode:    resp.StatusCode,
		ContentLength: resp.ContentLength,
		Headers:       headers,
		ProxyURL:      t.serverURL,
	}, nil
}

// roundTripViaTunnel 閫氳繃 sproxy 鍔犲瘑闅ч亾杞彂璇锋眰銆?func (t *SproxyTunnelTransport) roundTripViaTunnel(ctx context.Context, treq *download.TransportRequest) (*download.TransportResponse, error) {
	if err := isSafeTargetURL(treq.URL); err != nil {
		return nil, fmt.Errorf("sproxy: blocked unsafe URL: %w", err)
	}

	hreq, err := http.NewRequestWithContext(ctx, treq.Method, treq.URL, nil)
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
		headers[k] = resp.Header.Get(k)
	}
	return &download.TransportResponse{
		Body:          resp.Body,
		StatusCode:    resp.StatusCode,
		ContentLength: resp.ContentLength,
		Headers:       headers,
		ProxyURL:      t.serverURL,
	}, nil
}

// HealthCheck 妫€鏌?sproxy 鏈嶅姟鏄惁鍋ュ悍銆?func (t *SproxyTunnelTransport) HealthCheck(ctx context.Context) error {
	healthURL := t.serverURL + "/healthz"
	hreq, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return err
	}
	resp, err := t.client.Do(hreq)
	if err != nil {
		return fmt.Errorf("sproxy: health check failed: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sproxy: health check returned %d", resp.StatusCode)
	}
	return nil
}
