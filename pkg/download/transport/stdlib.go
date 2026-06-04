// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
)

// StdlibTransport 是基于标准库 net/http 的 Transport 实现。
type StdlibTransport struct {
	client   *http.Client
	dLimiter *download.DomainLimiter
}

// NewStdlibTransport 创建并返回一个 StdlibTransport 实例。
func NewStdlibTransport() *StdlibTransport {
	return &StdlibTransport{
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
		dLimiter: download.NewDomainLimiter(),
	}
}

// Name 返回传输层的名称。
func (t *StdlibTransport) Name() string { return "stdlib" }

// RoundTrip 实现 Transport 接口，执行一次 HTTP 往返。
func (t *StdlibTransport) RoundTrip(ctx context.Context, treq *download.TransportRequest) (*download.TransportResponse, error) {
	targetURL := treq.URL
	if treq.ProxyURL != "" {
		targetURL = strings.TrimPrefix(targetURL, "http://")
		targetURL = strings.TrimPrefix(targetURL, "https://")
		targetURL = treq.ProxyURL + "/" + targetURL
	}

	t.dLimiter.Acquire(treq.URL)
	defer t.dLimiter.Release(treq.URL)

	method := treq.Method
	if method == "" {
		method = "GET"
	}

	hreq, err := http.NewRequestWithContext(ctx, method, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range treq.Headers {
		hreq.Header.Set(k, v)
	}
	if treq.Range != nil && treq.Range.Offset > 0 {
		hreq.Header.Set("Range", fmt.Sprintf("bytes=%d-", treq.Range.Offset))
	}

	resp, err := t.client.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
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
		ProxyURL:      treq.ProxyURL,
	}, nil
}