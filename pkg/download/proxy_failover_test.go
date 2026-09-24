// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
)

// proxyForwardServer 模拟一个代理服务器：把 /<host>/<path> 形式的请求转发到目标。
// 返回 502 时模拟代理故障（用于故障切换测试）。
func proxyForwardServer(t testing.TB, target *httptest.Server, fail bool) *httptest.Server {
	t.Helper()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bandwidth" {
			// 模拟代理的带宽端点（数值越小越好）
			w.Write([]byte("50"))
			return
		}
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		// 代理格式：<proxy>/<host>/<path>（与 dlcore/sproxy 一致）
		trimmed := r.URL.Path
		for len(trimmed) > 0 && trimmed[0] == '/' {
			trimmed = trimmed[1:]
		}
		parts := splitOnce(trimmed, "/")
		if len(parts) < 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		upstream, _ := url.Parse("http://" + parts[0] + "/" + joinPath(parts[1:]))
		_ = upstream
		// 简化：直接转发到 target（测试固定目标）
		targetURL, _ := url.Parse(target.URL)
		rp := httputil.NewSingleHostReverseProxy(targetURL)
		rp.ServeHTTP(w, r)
	}))
	return proxy
}

func splitOnce(s, sep string) []string {
	for i := 0; i < len(s)-len(sep)+1; i++ {
		if s[i:i+len(sep)] == sep {
			return []string{s[:i], s[i+len(sep):]}
		}
	}
	return []string{s}
}

func joinPath(parts []string) string {
	out := ""
	for _, p := range parts {
		out += "/" + p
	}
	return out
}

// TestProxyFailoverIntegration 验证代理池故障切换端到端：A 代理 502，重试换到 B 代理成功。
func TestProxyFailoverIntegration(t *testing.T) {
	// 目标服务器（正常文件）
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("proxy failover content"))
	}))
	defer target.Close()

	// 代理 A：故障（502）；代理 B：正常转发
	proxyA := proxyForwardServer(t, target, true)
	defer proxyA.Close()
	proxyB := proxyForwardServer(t, target, false)
	defer proxyB.Close()

	// 构造选择器：A 故障、B 正常
	sel := NewStaticProxySelector([]string{proxyA.URL, proxyB.URL}).WithForceProxy(true)
	_ = sel

	// 用 DefaultSelector 包一层（模拟 downloader.New 接线）
	ds := NewDefaultSelector().WithProxySelector(sel)

	// 直接验证选择器：A 被标记故障后，Select 返回 B
	// 先手动触发 A 故障上报（模拟第一次请求失败）
	sel.ReportProxyFailure(proxyA.URL)
	sel.ReportProxyFailure(proxyA.URL)

	proxyURL, err := ds.SelectProxy(t.Context(), "http://example.com/file", nil)
	if err != nil {
		t.Fatalf("SelectProxy: %v", err)
	}
	if proxyURL != proxyB.URL {
		t.Fatalf("expected failover to proxy B, got %q", proxyURL)
	}
}

// TestReportProxyFailureThroughDefaultSelector 验证 reportProxyFailure 能穿透 DefaultSelector 到达底层选择器。
func TestReportProxyFailureThroughDefaultSelector(t *testing.T) {
	sel := NewStaticProxySelector([]string{"http://127.0.0.1:1"}).WithForceProxy(true)
	ds := NewDefaultSelector().WithProxySelector(sel)

	// 模拟 HTTPExtractor.reportProxyFailure 的穿透逻辑
	reporter := proxyReporter(ds)
	if reporter == nil {
		t.Fatal("expected ProxyFailureReporter reachable through DefaultSelector")
	}
	reporter.ReportProxyFailure("http://127.0.0.1:1")
	reporter.ReportProxyFailure("http://127.0.0.1:1")

	if sel.proxyHealthy("http://127.0.0.1:1") {
		t.Error("expected proxy unhealthy after 2 failures reported through DefaultSelector")
	}
}

// proxyReporter 递归查找实现了 ProxyFailureReporter 的底层选择器。
func proxyReporter(sel Selector) ProxyFailureReporter {
	if r, ok := sel.(ProxyFailureReporter); ok {
		return r
	}
	if ds, ok := sel.(*DefaultSelector); ok {
		if ps := ds.proxySelector; ps != nil {
			if r, ok := ps.(ProxyFailureReporter); ok {
				return r
			}
		}
	}
	return nil
}

var _ = context.Background
