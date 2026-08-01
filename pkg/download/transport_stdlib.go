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
	"time"
)

// StdlibTransport 是基于标准库 net/http 的 Transport 实现。
type StdlibTransport struct {
	client   *http.Client
	dLimiter *DomainLimiter
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
		dLimiter: NewDomainLimiter(),
	}
}

// Name 返回传输层的名称。
func (t *StdlibTransport) Name() string { return "stdlib" }

// RoundTrip 实现 Transport 接口，执行一次 HTTP 往返。
// treq 参数不能为 nil，否则返回错误。
func (t *StdlibTransport) RoundTrip(ctx context.Context, treq *TransportRequest) (*TransportResponse, error) {
	if treq == nil {
		return nil, fmt.Errorf("stdlib: nil TransportRequest")
	}
	targetURL := treq.URL
	var targetHost string

	if treq.ProxyURL != "" {
		// 使用 url.URL 结构体安全拼接代理 URL：
		// url.URL 已处理编码，Path 追加 u.Host + u.Path 是安全的。
		u, err := url.Parse(treq.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse target URL: %w", err)
		}
		proxyURL, err := url.Parse(treq.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse proxy URL: %w", err)
		}
		p := *proxyURL
		// TrimRight 防止 proxyURL.Path 以 "/" 结尾时产生双斜杠
		basePath := strings.TrimRight(proxyURL.Path, "/")
		p.Path = basePath + "/" + u.Host + u.Path
		p.RawQuery = u.RawQuery
		targetHost = u.Host
		targetURL = p.String()
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

	// 在代理模式下，显式设置 Host header 为目标主机，
	// 防止 http.NewRequestWithContext 将 Host 设为代理服务器地址。
	if targetHost != "" {
		hreq.Host = targetHost
	}

	resp, err := t.client.Do(hreq)
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

// CloseIdleConnections 关闭底层 http.Transport 的空闲连接。
func (t *StdlibTransport) CloseIdleConnections() {
	if tr, ok := t.client.Transport.(*http.Transport); ok {
		tr.CloseIdleConnections()
	}
}
