// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// StdlibTransport 是基于标准库 net/http 的 Transport 实现。
// 支持两种代理类型（见 ParseProxyKind）：
//   - ProxyKindStandard（无前缀，默认）：标准 HTTP 代理语义——http 目标发绝对 URI
//     （GET http://host/path），https 目标走 CONNECT 隧道（TLS 端到端）。与 curl /
//     http.Transport.Proxy 一致，兼容 sproxy http-proxy 与常见代理服务器。
//   - ProxyKindGateway（"gateway:" 前缀）：网关式转发（旧行为）——把目标域名拼进
//     代理 URL 路径（http://proxy/<host>/<path>），适用于旧式网关代理。
type StdlibTransport struct {
	client   *http.Client
	dLimiter *DomainLimiter
	// proxyClients 缓存带标准代理的 http.Client（key = 剥离前缀后的代理 URL），
	// 供 ProxyKindStandard 路径使用（连接池复用，HLS 分片场景友好）。
	proxyMu      sync.Mutex
	proxyClients map[string]*http.Client
}

// NewStdlibTransport 创建并返回一个 StdlibTransport 实例。
func NewStdlibTransport() *StdlibTransport {
	return &StdlibTransport{
		client: &http.Client{
			// 不使用全局 Timeout，拆分为连接超时 + 响应头超时，
			// 避免大文件下载被 5 分钟超时截断。
			Transport: &http.Transport{
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   10,
				IdleConnTimeout:       30 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
			},
		},
		dLimiter:     NewDomainLimiter(),
		proxyClients: make(map[string]*http.Client),
	}
}

// Name 返回传输层的名称。
func (t *StdlibTransport) Name() string { return "stdlib" }

// proxyClientFor 返回使用指定标准代理的 http.Client（连接池按代理 URL 缓存）。
// proxyURL 必须已剥离类型前缀。
func (t *StdlibTransport) proxyClientFor(proxyURL string) *http.Client {
	t.proxyMu.Lock()
	defer t.proxyMu.Unlock()
	if c, ok := t.proxyClients[proxyURL]; ok {
		return c
	}
	pu, err := url.Parse(proxyURL)
	if err != nil {
		return nil
	}
	// 基座克隆：保留超时/连接池配置，仅覆写 Proxy。
	base := t.client.Transport.(*http.Transport).Clone()
	base.Proxy = http.ProxyURL(pu)
	c := &http.Client{Transport: base}
	t.proxyClients[proxyURL] = c
	return c
}

// RoundTrip 实现 Transport 接口，执行一次 HTTP 往返。
// treq 参数不能为 nil，否则返回错误。
func (t *StdlibTransport) RoundTrip(ctx context.Context, treq *TransportRequest) (*TransportResponse, error) {
	if treq == nil {
		return nil, fmt.Errorf("stdlib: nil TransportRequest")
	}
	kind, rawProxy := ParseProxyKind(treq.ProxyURL)

	var (
		targetURL  = treq.URL
		targetHost string
		useClient  = t.client
	)

	if rawProxy != "" {
		switch kind {
		case ProxyKindGateway:
			// 网关式（旧行为）：把目标域名拼进代理 URL 路径。
			u, err := url.Parse(treq.URL)
			if err != nil {
				return nil, fmt.Errorf("failed to parse target URL: %w", err)
			}
			proxyURL, err := url.Parse(rawProxy)
			if err != nil {
				return nil, fmt.Errorf("failed to parse proxy URL: %w", err)
			}
			p := *proxyURL
			basePath := strings.TrimRight(proxyURL.Path, "/")
			p.Path = basePath + "/" + u.Host + u.Path
			p.RawQuery = u.RawQuery
			targetHost = u.Host
			targetURL = p.String()
		default:
			// 标准代理：原样目标 URL，http.Transport.Proxy 自动发绝对 URI / CONNECT。
			if pc := t.proxyClientFor(rawProxy); pc != nil {
				useClient = pc
			}
		}
	}

	if err := t.dLimiter.Acquire(ctx, treq.URL); err != nil {
		return nil, fmt.Errorf("domain limiter acquire: %w", err)
	}
	defer t.dLimiter.Release(treq.URL)

	method := treq.Method
	if method == "" {
		method = "GET"
	}

	var body io.Reader
	if len(treq.Body) > 0 {
		body = bytes.NewReader(treq.Body)
	}
	hreq, err := http.NewRequestWithContext(ctx, method, targetURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range treq.Headers {
		hreq.Header.Set(k, v)
	}
	if treq.Range != nil && treq.Range.Offset > 0 {
		hreq.Header.Set("Range", fmt.Sprintf("bytes=%d-", treq.Range.Offset))
	}

	// 网关式时显式设置 Host header 为目标主机，
	// 防止 http.NewRequestWithContext 将 Host 设为代理服务器地址。
	// 标准代理路径由 Transport.Proxy 处理，无需覆盖。
	if targetHost != "" {
		hreq.Host = targetHost
	}

	resp, err := useClient.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	headers := make(map[string]string)
	for k := range resp.Header {
		headers[k] = strings.Join(resp.Header.Values(k), ", ")
	}

	return &TransportResponse{
		Body:          resp.Body,
		StatusCode:    resp.StatusCode,
		ContentLength: resp.ContentLength,
		Headers:       headers,
		ProxyURL:      treq.ProxyURL,
	}, nil
}

// SetDomainLimits 设置域名并发限制。
func (t *StdlibTransport) SetDomainLimits(limits map[string]int) {
	for domain, limit := range limits {
		t.dLimiter.Set(domain, limit)
	}
}

// RemoveDomainLimits 删除指定域名的并发限制并唤醒所有等待者。
func (t *StdlibTransport) RemoveDomainLimits(domains []string) {
	for _, domain := range domains {
		t.dLimiter.Remove(domain)
	}
}

// Remove 删除指定域名的并发限制。
func (t *StdlibTransport) Remove(domain string) {
	t.dLimiter.Remove(domain)
}

// CloseIdleConnections 关闭底层 http.Transport 的空闲连接。
func (t *StdlibTransport) CloseIdleConnections() {
	if tr, ok := t.client.Transport.(*http.Transport); ok {
		tr.CloseIdleConnections()
	}
}
