// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// startRecordProxy 记录收到的请求形态，模拟标准 HTTP 代理。
// 返回：代理地址 + 记录（绝对 URI / CONNECT）。
func startRecordProxy(t *testing.T) (string, *atomic.Int64, *atomic.Int64) {
	t.Helper()
	var absURICount, connectCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			connectCount.Add(1)
			hij, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "no hijack", http.StatusInternalServerError)
				return
			}
			conn, _, err := hij.Hijack()
			if err != nil {
				return
			}
			// 模拟 CONNECT 目标（回 200 + 原样回显几个字节）
			_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
			_ = conn.Close()
			return
		}
		absURICount.Add(1)
		// 标准代理：请求行应为绝对 URI（r.RequestURI 含 scheme://host）
		if !strings.HasPrefix(r.RequestURI, "http://") && !strings.HasPrefix(r.RequestURI, "https://") {
			t.Errorf("标准代理收到非绝对 URI: %q", r.RequestURI)
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &absURICount, &connectCount
}

// TestStdlibTransport_StandardProxyAbsoluteURI 验证无前缀代理 = 标准语义：
// http 目标发绝对 URI（请求行带 scheme://host）。
func TestStdlibTransport_StandardProxyAbsoluteURI(t *testing.T) {
	t.Parallel()
	proxyAddr, absCount, _ := startRecordProxy(t)

	tr := NewStdlibTransport()
	// 目标：代理服务器自身即可（验证请求行形态）
	target := strings.TrimPrefix(proxyAddr, "http://")
	_ = target
	// 用一个真实可访问的 http 目标（代理后面向任意 host 转发）——
	// 直接指向代理自身地址作为目标 host，验证请求行是绝对 URI。
	treq := &TransportRequest{
		URL:      proxyAddr + "/file.bin",
		Method:   http.MethodGet,
		ProxyURL: proxyAddr,
	}
	resp, err := tr.RoundTrip(context.Background(), treq)
	if err != nil {
		t.Fatalf("RoundTrip err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if absCount.Load() == 0 {
		t.Error("标准代理未收到绝对 URI 请求")
	}
}

// TestStdlibTransport_GatewayProxyPath 验证 gateway: 前缀 = 旧网关式：
// 把目标域名拼进 URL 路径。
func TestStdlibTransport_GatewayProxyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 网关式：代理收到的是 origin-form 请求（路径含目标域名）
		if r.RequestURI != "/example.com/file.bin" {
			t.Errorf("网关式路径 = %q, want /example.com/file.bin", r.RequestURI)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("gateway-ok"))
	}))
	t.Cleanup(srv.Close)

	tr := NewStdlibTransport()
	treq := &TransportRequest{
		URL:      "http://example.com/file.bin",
		Method:   http.MethodGet,
		ProxyURL: "gateway:" + srv.URL,
	}
	resp, err := tr.RoundTrip(context.Background(), treq)
	if err != nil {
		t.Fatalf("RoundTrip err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
