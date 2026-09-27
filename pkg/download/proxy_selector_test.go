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

// ---- StaticProxySelector ----

func TestStaticProxySelectorNoProxies(t *testing.T) {
	tests := []struct {
		name    string
		proxies []string
	}{
		{name: "nil", proxies: nil},
		{name: "empty", proxies: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStaticProxySelector(tt.proxies)
			proxy, err := s.Select(t.Context(), "http://example.com/file.zip", nil)
			if err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
			if proxy != "" {
				t.Errorf("expected empty proxy (direct), got: %s", proxy)
			}
		})
	}
}

func TestStaticProxySelectorWithForceProxy(t *testing.T) {
	s := NewStaticProxySelector([]string{"http://127.0.0.1:1"}).WithForceProxy(true)
	proxy, err := s.Select(t.Context(), "http://example.com/file.zip", nil)
	if err == nil {
		t.Error("expected error when forceProxy and no proxy available")
	}
	if proxy != "" {
		t.Errorf("expected empty proxy on error, got: %s", proxy)
	}
}

// ---- DefaultSelector ----

func TestDefaultSelectorSelectProxy(t *testing.T) {
	t.Run("with proxy selector", func(t *testing.T) {
		mockPS := &mockProxySelector{proxyURL: "http://test-proxy:8080"}
		sel := NewDefaultSelector().WithProxySelector(mockPS)
		proxy, err := sel.SelectProxy(t.Context(), "http://example.com/file", nil)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if proxy != "http://test-proxy:8080" {
			t.Errorf("expected proxy http://test-proxy:8080, got: %s", proxy)
		}
	})

	t.Run("without proxy selector", func(t *testing.T) {
		sel := NewDefaultSelector()
		proxy, err := sel.SelectProxy(t.Context(), "http://example.com/file", nil)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if proxy != "" {
			t.Errorf("expected empty proxy, got: %s", proxy)
		}
	})
}

type mockProxySelector struct {
	proxyURL string
}

func (m *mockProxySelector) Select(_ context.Context, _ string, _ *DownloadHint) (string, error) {
	return m.proxyURL, nil
}

// ---- 代理池轮换与故障切换 ----

// mockBandwidthServer 返回一个返回固定带宽值的代理服务器。
func mockBandwidthServer(t testing.TB, bandwidth string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bandwidth" {
			w.Write([]byte(bandwidth))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

// TestStaticProxySelectorCooldown 验证代理故障后进入冷却，后续 Select 跳过它。
func TestStaticProxySelectorCooldown(t *testing.T) {
	alive := mockBandwidthServer(t, "10")
	defer alive.Close()
	dead := mockBandwidthServer(t, "not-a-number") // 带宽探测失败 → 永不选中（getProxyBandwidth 返回 MaxFloat64）

	s := NewStaticProxySelector([]string{dead.URL, alive.URL}).WithForceProxy(true)
	// 冷却前：能选出（alive 带宽小）
	p, err := s.Select(t.Context(), "http://example.com/file", nil)
	if err != nil || p != alive.URL {
		t.Fatalf("expected alive proxy selected, got %q err=%v", p, err)
	}

	// 标记 alive 失败两次 → 进入冷却
	s.ReportProxyFailure(alive.URL)
	s.ReportProxyFailure(alive.URL)
	// alive 冷却中，dead 带宽差 → 无可用 → forceProxy 返回错误
	p, err = s.Select(t.Context(), "http://example.com/file", nil)
	if err == nil {
		t.Fatalf("expected error when all proxies in cooldown, got proxy %q", p)
	}
}

// TestStaticProxySelectorRotation 验证轮换：多次 Select 会轮换起始代理。
func TestStaticProxySelectorRotation(t *testing.T) {
	p1 := mockBandwidthServer(t, "1")
	defer p1.Close()
	p2 := mockBandwidthServer(t, "2")
	defer p2.Close()
	p3 := mockBandwidthServer(t, "3")
	defer p3.Close()

	s := NewStaticProxySelector([]string{p1.URL, p2.URL, p3.URL}).WithForceProxy(true)
	seen := make(map[string]int)
	for range 9 {
		p, err := s.Select(t.Context(), "http://example.com/file", nil)
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		seen[p]++
	}
	// 轮换应让多个代理被选到（轮换游标改变起点 + 带宽评分）
	if len(seen) < 2 {
		t.Errorf("expected rotation across proxies, seen=%v", seen)
	}
}

// TestStaticProxySelectorFailover 验证故障切换：A 故障冷却后，请求切到 B。
func TestStaticProxySelectorFailover(t *testing.T) {
	proxyA := mockBandwidthServer(t, "1") // 带宽最优，但会被标记故障
	defer proxyA.Close()
	proxyB := mockBandwidthServer(t, "2")
	defer proxyB.Close()

	s := NewStaticProxySelector([]string{proxyA.URL, proxyB.URL}).WithForceProxy(true)

	// 初始选中 A（带宽最优）
	p, err := s.Select(t.Context(), "http://example.com/file", nil)
	if err != nil || p != proxyA.URL {
		t.Fatalf("expected A selected first, got %q err=%v", p, err)
	}

	// A 失败两次 → 冷却
	s.ReportProxyFailure(proxyA.URL)
	s.ReportProxyFailure(proxyA.URL)

	// 后续请求切到 B
	p, err = s.Select(t.Context(), "http://example.com/file", nil)
	if err != nil || p != proxyB.URL {
		t.Fatalf("expected B selected after A cooldown, got %q err=%v", p, err)
	}
}

// TestCheckDirect_UsesBrowserUA 验证直连探测带浏览器 UA（CF 等站点对无 UA 请求返回 403，
// 会导致直连误判失败 → 走代理 → 代理 IP 又被 CF challenge → 悬挂）。
func TestCheckDirect_UsesBrowserUA(t *testing.T) {
	t.Parallel()
	var gotUA atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA.Store(r.Header.Get("User-Agent"))
		// 模拟 CF：无 UA 或默认 Go UA 返回 403，浏览器 UA 返回 200。
		ua := r.Header.Get("User-Agent")
		if ua == "" || strings.HasPrefix(ua, "Go-http-client") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if !checkDirect(t.Context(), srv.URL, 2) {
		t.Fatal("checkDirect 应返回 true（带浏览器 UA 的请求被放行）")
	}
	if ua, _ := gotUA.Load().(string); ua == "" || strings.HasPrefix(ua, "Go-http-client") {
		t.Fatalf("checkDirect 未带浏览器 UA: %q", ua)
	}
}
