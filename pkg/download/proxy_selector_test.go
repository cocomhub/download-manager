// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- StaticProxySelector ----

func TestStaticProxySelectorNoProxies(t *testing.T) {
	s := NewStaticProxySelector(nil)
	proxy, err := s.Select(t.Context(), "http://example.com/file.zip", nil)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if proxy != "" {
		t.Errorf("expected empty proxy (direct), got: %s", proxy)
	}
}

func TestStaticProxySelectorEmptyProxies(t *testing.T) {
	s := NewStaticProxySelector([]string{})
	proxy, err := s.Select(t.Context(), "http://example.com/file.zip", nil)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if proxy != "" {
		t.Errorf("expected empty proxy (direct), got: %s", proxy)
	}
}

func TestStaticProxySelectorWithForceProxy(t *testing.T) {
	s := NewStaticProxySelector([]string{"http://127.0.0.1:1"})
	s.forceProxy = true
	proxy, err := s.Select(t.Context(), "http://example.com/file.zip", nil)
	if err == nil {
		t.Error("expected error when forceProxy and no proxy available")
	}
	if proxy != "" {
		t.Errorf("expected empty proxy on error, got: %s", proxy)
	}
}

func TestDefaultMaxBandwidthConstant(t *testing.T) {
	if defaultMaxBandwidth != 999999.0 {
		t.Errorf("expected defaultMaxBandwidth to be 999999.0, got %f", defaultMaxBandwidth)
	}
}

func TestStaticProxySelectorCacheDir(t *testing.T) {
	mockProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/bandwidth") {
			w.Write([]byte("50.0"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer mockProxy.Close()

	cacheDir := t.TempDir() + "/proxy_cache"
	s := NewStaticProxySelector([]string{mockProxy.URL})
	s.forceProxy = true
	s.cacheDir = cacheDir

	proxy, err := s.Select(t.Context(), "http://example.com/file.zip", nil)
	if err != nil {
		t.Fatalf("Select should succeed with mock proxy: %v", err)
	}
	if proxy != mockProxy.URL {
		t.Errorf("expected proxy URL %s, got %s", mockProxy.URL, proxy)
	}

	// 验证缓存文件创建
	domain := "example.com"
	cachePath := filepath.Join(cacheDir, domain)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("cache file should exist: %v", err)
	}
	if string(data) != "proxy" {
		t.Errorf("expected cache content 'proxy', got %q", string(data))
	}
}

// ---- getProxyBandwidth ----

func TestGetProxyBandwidth(t *testing.T) {
	mockProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/bandwidth") {
			w.Write([]byte("150.5"))
			return
		}
	}))
	defer mockProxy.Close()

	bw := getProxyBandwidth(t.Context(), mockProxy.URL, "/bandwidth", 3)
	if bw != 150.5 {
		t.Errorf("expected 150.5, got %f", bw)
	}
}

func TestGetProxyBandwidthOnFailure(t *testing.T) {
	bw := getProxyBandwidth(t.Context(), "http://127.0.0.1:1", "/bandwidth", 1)
	if bw != defaultMaxBandwidth {
		t.Errorf("expected defaultMaxBandwidth(%f) on failure, got %f", defaultMaxBandwidth, bw)
	}
}

// ---- checkDirect ----

func TestCheckDirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if !checkDirect(t.Context(), srv.URL, 3) {
		t.Error("checkDirect should return true for healthy server")
	}
}

func TestCheckDirectOnUnreachable(t *testing.T) {
	if checkDirect(t.Context(), "http://127.0.0.1:1", 1) {
		t.Error("checkDirect should return false for unreachable server")
	}
}
