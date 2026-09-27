// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestScraperFetch_Direct 直连抓取：返回 HTML + 带浏览器 UA。
func TestScraperFetch_Direct(t *testing.T) {
	t.Parallel()
	var gotUA atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA.Store(r.Header.Get("User-Agent"))
		_, _ = w.Write([]byte("<html>direct ok</html>"))
	}))
	defer srv.Close()

	s := newScraper(scraperConfig{})
	body, err := s.fetch(srv.URL)
	if err != nil {
		t.Fatalf("fetch err: %v", err)
	}
	if !strings.Contains(body, "direct ok") {
		t.Errorf("body = %q", body)
	}
	if ua, _ := gotUA.Load().(string); !strings.HasPrefix(ua, "Mozilla/5.0") {
		t.Errorf("UA = %q, want browser UA", ua)
	}
}

// TestScraperFetch_StandardProxy 标准代理（无前缀）：http.Transport.Proxy 转发。
func TestScraperFetch_StandardProxy(t *testing.T) {
	t.Parallel()
	var gotRequestURI atomic.Value
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI.Store(r.RequestURI)
		_, _ = w.Write([]byte("proxied"))
	}))
	defer proxy.Close()

	s := newScraper(scraperConfig{proxyURL: proxy.URL})
	// 目标用代理自身地址（验证请求形态：标准代理发绝对 URI）。
	body, err := s.fetch(proxy.URL + "/file.txt")
	if err != nil {
		t.Fatalf("fetch err: %v", err)
	}
	if !strings.Contains(body, "proxied") {
		t.Errorf("body = %q", body)
	}
	uri, _ := gotRequestURI.Load().(string)
	if !strings.HasPrefix(uri, "http://") {
		t.Errorf("标准代理请求应绝对 URI, got %q", uri)
	}
}

// TestScraperFetch_GatewayProxy gateway: 前缀拼 URL（旧行为兼容）。
func TestScraperFetch_GatewayProxy(t *testing.T) {
	t.Parallel()
	var gotPath atomic.Value
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath.Store(r.RequestURI)
		_, _ = w.Write([]byte("gw"))
	}))
	defer proxy.Close()

	s := newScraper(scraperConfig{proxyURL: "gateway:" + proxy.URL})
	_, err := s.fetch("https://example.com/file.txt")
	if err != nil {
		t.Fatalf("fetch err: %v", err)
	}
	p, _ := gotPath.Load().(string)
	if !strings.Contains(p, "example.com/file.txt") {
		t.Errorf("gateway 拼 URL 缺域名: %q", p)
	}
}

// TestScraperFetch_Timeout 超时：慢服务器 → 快速失败（默认 30s，测试用 1s）。
func TestScraperFetch_Timeout(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second) // 慢响应（超过 client timeout）
		_, _ = w.Write([]byte("late"))
	}))
	defer srv.Close()

	s := newScraper(scraperConfig{timeout: 1})
	_, err := s.fetch(srv.URL)
	if err == nil {
		t.Fatal("应超时失败")
	}
	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "deadline") &&
		!strings.Contains(err.Error(), "Client.Timeout") {
		t.Errorf("错误应为超时: %v", err)
	}
}
