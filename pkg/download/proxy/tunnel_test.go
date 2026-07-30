// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download/proxy"
)

func TestTunnelProxySelectorNoInstances(t *testing.T) {
	sel := proxy.NewTunnelProxySelector()
	proxyURL, err := sel.Select(t.Context(), "http://example.com/file", nil)
	if err != nil {
		t.Fatalf("Select with no instances should not error: %v", err)
	}
	if proxyURL != "" {
		t.Errorf("expected empty proxy, got %s", proxyURL)
	}
}

func TestTunnelProxySelectorWithInstance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/healthz") {
			w.WriteHeader(http.StatusOK)
		} else if strings.HasSuffix(r.URL.Path, "/bandwidth") {
			// 返回足够的数据用于带宽探测
			w.Write([]byte(strings.Repeat("x", 512*1024)))
		}
	}))
	defer srv.Close()

	sel := proxy.NewTunnelProxySelector(
		proxy.WithTunnelInstance(srv.URL, "0000000000000000000000000000000000000000000000000000000000000000"),
	)
	proxyURL, err := sel.Select(t.Context(), "http://example.com/file", nil)
	if err != nil {
		t.Fatalf("Select should not error: %v", err)
	}
	if proxyURL == "" {
		t.Error("expected a proxy URL, got empty")
	}
	if proxyURL != srv.URL {
		t.Errorf("expected proxy %s, got %s", srv.URL, proxyURL)
	}
}

func TestTunnelProxySelectorMultipleInstances(t *testing.T) {
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/healthz") {
			w.WriteHeader(http.StatusOK)
		} else if strings.HasSuffix(r.URL.Path, "/bandwidth") {
			w.Write([]byte(strings.Repeat("x", 512*1024)))
		}
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/healthz") {
			w.WriteHeader(http.StatusOK)
		} else if strings.HasSuffix(r.URL.Path, "/bandwidth") {
			w.Write([]byte(strings.Repeat("x", 512*1024)))
		}
	}))
	defer srv2.Close()

	sel := proxy.NewTunnelProxySelector(
		proxy.WithTunnelInstance(srv1.URL, "key1"),
		proxy.WithTunnelInstance(srv2.URL, "key2"),
	)
	proxyURL, err := sel.Select(t.Context(), "http://example.com/file", nil)
	if err != nil {
		t.Fatalf("Select should not error: %v", err)
	}
	if proxyURL == "" {
		t.Fatal("expected a proxy URL, got empty")
	}
	// 两个实例都健康且有带宽，应该返回其中一个
	if proxyURL != srv1.URL && proxyURL != srv2.URL {
		t.Errorf("expected proxy to be one of the test servers, got %s", proxyURL)
	}
}

func TestTunnelProxySelectorAllUnavailable(t *testing.T) {
	// 所有实例健康检查失败
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv1.Close()

	sel := proxy.NewTunnelProxySelector(
		proxy.WithTunnelInstance(srv1.URL, "key1"),
	)
	proxyURL, err := sel.Select(t.Context(), "http://example.com/file", nil)
	if err != nil {
		t.Fatalf("Select should not error when all unavailable: %v", err)
	}
	if proxyURL != "" {
		t.Errorf("expected empty proxy URL when all unavailable, got %s", proxyURL)
	}
}
