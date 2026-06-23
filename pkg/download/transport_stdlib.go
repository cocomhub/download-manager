// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// StdlibTransport 鏄熀浜庢爣鍑嗗簱 net/http 鐨?Transport 瀹炵幇銆?type StdlibTransport struct {
	client   *http.Client
	dLimiter *DomainLimiter
}

// NewStdlibTransport 鍒涘缓骞惰繑鍥炰竴涓?StdlibTransport 瀹炰緥銆?func NewStdlibTransport() *StdlibTransport {
	return &StdlibTransport{
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
		dLimiter: NewDomainLimiter(),
	}
}

// Name 杩斿洖浼犺緭灞傜殑鍚嶇О銆?func (t *StdlibTransport) Name() string { return "stdlib" }

// RoundTrip 瀹炵幇 Transport 鎺ュ彛锛屾墽琛屼竴娆?HTTP 寰€杩斻€?func (t *StdlibTransport) RoundTrip(ctx context.Context, treq *TransportRequest) (*TransportResponse, error) {
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

	return &TransportResponse{
		Body:          resp.Body,
		StatusCode:    resp.StatusCode,
		ContentLength: resp.ContentLength,
		Headers:       headers,
		ProxyURL:      treq.ProxyURL,
	}, nil
}

// SetDomainLimits 璁剧疆鍩熷悕骞跺彂闄愬埗銆?func (t *StdlibTransport) SetDomainLimits(limits map[string]int) {
	for domain, limit := range limits {
		t.dLimiter.Set(domain, limit)
	}
}
