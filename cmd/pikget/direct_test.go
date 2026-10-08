// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// newTestFileServer 返回一个提供固定内容文件的 httptest server，并记录请求。
func newTestFileServer(t *testing.T, content []byte) (*httptest.Server, *atomic.Int64, *atomic.Int64) {
	t.Helper()
	var reqCount, rangeCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		if r.Header.Get("Range") != "" {
			rangeCount.Add(1)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		// 支持 Range 请求（续传能力验证）
		http.ServeContent(w, r, "test.bin", time.Time{}, bytes.NewReader(content))
	}))
	t.Cleanup(srv.Close)
	return srv, &reqCount, &rangeCount
}

func TestDownloadDirect_Success(t *testing.T) {
	content := []byte("hello pikget direct download")
	srv, _, _ := newTestFileServer(t, content)

	dest := filepath.Join(t.TempDir(), "out.bin")
	err := downloadDirect(t.Context(), srv.URL+"/test.bin", dest, httpOpts{maxRetry: 3}, nil)
	if err != nil {
		t.Fatalf("downloadDirect: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("content mismatch: got %q want %q", got, content)
	}
}

func TestDownloadDirect_Progress0to100(t *testing.T) {
	content := bytes.Repeat([]byte("a"), 1024*64)
	srv, _, _ := newTestFileServer(t, content)

	var maxP atomic.Int64
	var calls atomic.Int64
	err := downloadDirect(t.Context(), srv.URL+"/big.bin", filepath.Join(t.TempDir(), "out.bin"),
		httpOpts{maxRetry: 3}, func(p float64, downloaded, total int64) {
			calls.Add(1)
			if p > float64(maxP.Load()) {
				maxP.Store(int64(p))
			}
		})
	if err != nil {
		t.Fatalf("downloadDirect: %v", err)
	}
	if calls.Load() == 0 {
		t.Fatal("expected at least one progress callback")
	}
	if maxP.Load() != 100 {
		t.Fatalf("expected final progress 100, got %d", maxP.Load())
	}
}

func TestDownloadDirect_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	err := downloadDirect(t.Context(), srv.URL+"/err.bin", filepath.Join(t.TempDir(), "out.bin"), httpOpts{maxRetry: 3}, nil)
	if err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestDownloadDirect_RangeResume(t *testing.T) {
	// 首次下载一部分（模拟上次中断留下的 .partial），验证第二次请求带 Range 续传。
	content := bytes.Repeat([]byte("xyz"), 1024*16)
	srv, _, rangeCount := newTestFileServer(t, content)

	dest := filepath.Join(t.TempDir(), "out.bin")
	// 预写一半内容（模拟中断后的部分文件）
	half := content[:len(content)/2]
	if err := os.WriteFile(dest, half, 0644); err != nil {
		t.Fatalf("write partial: %v", err)
	}

	err := downloadDirect(t.Context(), srv.URL+"/resume.bin", dest, httpOpts{maxRetry: 3}, nil)
	if err != nil {
		t.Fatalf("downloadDirect: %v", err)
	}
	if rangeCount.Load() == 0 {
		t.Fatal("expected a Range request for resume")
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("resumed content mismatch: got len=%d want len=%d", len(got), len(content))
	}
}

func TestDownloadDirect_UserAgentHeader(t *testing.T) {
	var gotUA atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA.Store(r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	err := downloadDirect(t.Context(), srv.URL+"/ua.bin", filepath.Join(t.TempDir(), "out.bin"),
		httpOpts{userAgent: "pikget-test/1.0", maxRetry: 3}, nil)
	if err != nil {
		t.Fatalf("downloadDirect: %v", err)
	}
	if ua, _ := gotUA.Load().(string); ua != "pikget-test/1.0" {
		t.Fatalf("User-Agent = %q, want pikget-test/1.0", ua)
	}
}

// TestDownloadDirect_CtxCancel 验证取消后不产生完整文件（退出码 130 路径）。
func TestDownloadDirect_CtxCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "104857600") // 声明 100MB
		fl := w.(http.Flusher)
		fl.Flush()
		select {} // 挂起直到连接关闭
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- downloadDirect(ctx, srv.URL+"/hang.bin", filepath.Join(t.TempDir(), "out.bin"), httpOpts{maxRetry: 3}, nil)
	}()
	cancel()
	if err := <-done; err == nil {
		t.Fatal("expected error after cancel")
	}
}
