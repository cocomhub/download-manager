// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestUI 构造 UI handler（复用 buildHandler），backend 为模拟下载进程。
func newTestUI(t *testing.T, backend *httptest.Server) http.Handler {
	t.Helper()
	h, err := buildHandler(backend.URL)
	if err != nil {
		t.Fatalf("buildHandler: %v", err)
	}
	return h
}

func TestUI_ServesStaticIndex(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("backend should not be called for static /, got %s", r.URL.Path)
	}))
	defer backend.Close()

	h := newTestUI(t, backend)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "<!DOCTYPE html") {
		t.Errorf("body should contain HTML, got: %s", rr.Body.String()[:100])
	}
}

func TestUI_ProxiesAPIAndSetsForwardedHost(t *testing.T) {
	var gotHost, gotFwdHost, gotAuth, gotOrigin string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotFwdHost = r.Header.Get("X-Forwarded-Host")
		gotAuth = r.Header.Get("Authorization")
		gotOrigin = r.Header.Get("Origin")
		w.Header().Set("X-Backend", "yes")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer backend.Close()

	h := newTestUI(t, backend)
	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	req.Host = "localhost:9000"
	req.Header.Set("Authorization", "Bearer abc")
	req.Header.Set("Origin", "http://localhost:9000")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if rr.Header().Get("X-Backend") != "yes" {
		t.Errorf("backend response header not propagated")
	}
	if gotAuth != "Bearer abc" {
		t.Errorf("Authorization = %q, want Bearer abc (透传)", gotAuth)
	}
	if gotOrigin != "http://localhost:9000" {
		t.Errorf("Origin = %q, want http://localhost:9000 (透传)", gotOrigin)
	}
	if gotFwdHost != "localhost:9000" {
		t.Errorf("X-Forwarded-Host = %q, want localhost:9000", gotFwdHost)
	}
	if gotHost == "localhost:9000" {
		t.Errorf("Host should be rewritten to backend, got %q", gotHost)
	}
}

func TestUI_ProxiesFilesPrefix(t *testing.T) {
	var gotPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "video/mp4")
		w.Write([]byte("fake-video"))
	}))
	defer backend.Close()

	h := newTestUI(t, backend)
	req := httptest.NewRequest(http.MethodGet, "/files/njavtv/abc.mp4", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if gotPath != "/files/njavtv/abc.mp4" {
		t.Errorf("backend path = %q, want /files/njavtv/abc.mp4", gotPath)
	}
}

func TestUI_ProxiesSSE(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// 立即返回（SSE 长连接在测试中只验证头 + 首块）
		io.WriteString(w, "data: {\"type\":\"task_update\"}\n\n")
	}))
	defer backend.Close()

	h := newTestUI(t, backend)
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	req.Header.Set("Origin", "http://localhost:9000")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
}
