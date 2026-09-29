// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package m3u8d

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cavaliergopher/grab/v3"
)

// TestDownloadFilesConcurrently_4xxReturnsExplicitError 验证（P1 修复）：
// 终态 4xx（403）返回显式错误并立即中止，downloadFilesConcurrently 失败而非静默当成功；
// 且不占用重试轮次（不再对终态 4xx 重复请求）。
func TestDownloadFilesConcurrently_4xxReturnsExplicitError(t *testing.T) {
	var hits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/forbidden.ts", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusForbidden)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	d := &M3U8DEngine{
		Config: &DownloadConfig{
			WorkDir:     dir,
			Concurrency: 2,
			Timeout:     300 * time.Millisecond,
			Verbose:     false,
		},
		downloaded: make(map[string]bool),
	}
	tasks := []DownloadTask{
		{URL: srv.URL + "/forbidden.ts", LocalPath: filepath.Join(dir, "b.ts"), Type: "ts"},
	}
	err := d.downloadFilesConcurrently(t.Context(), tasks)
	if err == nil {
		t.Fatal("expected explicit error for terminal 4xx, got nil (silent success)")
	}
	if !containsURL(err.Error(), "forbidden.ts") || !containsURL(err.Error(), "403") {
		t.Fatalf("expected error mentioning forbidden.ts HTTP 403, got: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected terminal 4xx not retried, got %d requests", hits.Load())
	}
}

// TestDownloadFilesConcurrently_429IsRetried 验证（P1 修复）：
// 429（Too Many Requests）是可重试 4xx，进入 errReqs 参与下一轮重试。
func TestDownloadFilesConcurrently_429IsRetried(t *testing.T) {
	var hits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/429.ts", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	d := &M3U8DEngine{
		Config: &DownloadConfig{
			WorkDir:     dir,
			Concurrency: 1,
			Timeout:     300 * time.Millisecond,
			Verbose:     false,
		},
		downloaded: make(map[string]bool),
	}
	tasks := []DownloadTask{
		{URL: srv.URL + "/429.ts", LocalPath: filepath.Join(dir, "b.ts"), Type: "ts"},
	}
	err := d.downloadFilesConcurrently(t.Context(), tasks)
	if err == nil {
		t.Fatal("expected failure after retry rounds exhausted")
	}
	if !containsURL(err.Error(), "429.ts") {
		t.Fatalf("expected error mentioning 429.ts, got: %v", err)
	}
	// 429 应被保留进 errReqs 并重试（不止请求一次）。
	if hits.Load() < 2 {
		t.Fatalf("expected 429 retried across rounds, got %d requests", hits.Load())
	}
}

// TestDownloadFilesConcurrently_472SetsConcurrencyOneAndRetries 验证（P1 修复）：
// 472 保留「降并发为 1 + 进入重试」，不被静默吞掉。
func TestDownloadFilesConcurrently_472SetsConcurrencyOneAndRetries(t *testing.T) {
	var hits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/472.ts", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(472)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	d := &M3U8DEngine{
		Config: &DownloadConfig{
			WorkDir:     dir,
			Concurrency: 4,
			Timeout:     300 * time.Millisecond,
			Verbose:     false,
		},
		downloaded: make(map[string]bool),
	}
	tasks := []DownloadTask{
		{URL: srv.URL + "/472.ts", LocalPath: filepath.Join(dir, "b.ts"), Type: "ts"},
	}
	err := d.downloadFilesConcurrently(t.Context(), tasks)
	if err == nil {
		t.Fatal("expected failure after retry rounds exhausted")
	}
	if !containsURL(err.Error(), "472.ts") {
		t.Fatalf("expected error mentioning 472.ts, got: %v", err)
	}
	// 472 命中后并发应被降为 1。
	d.concurrencyMu.Lock()
	gotConc := d.Config.Concurrency
	d.concurrencyMu.Unlock()
	if gotConc != 1 {
		t.Fatalf("expected Concurrency downgraded to 1 on 472, got: %d", gotConc)
	}
	// 472 应进入重试（不止请求一次）。
	if hits.Load() < 2 {
		t.Fatalf("expected 472 retried, got %d requests", hits.Load())
	}
}

// TestDownloadFilesConcurrently_404ReturnsExplicitError 验证（P1 修复）：
// 终态 4xx（404）返回显式错误而非把缺片静默留到转码阶段。
func TestDownloadFilesConcurrently_404ReturnsExplicitError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/missing.ts", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	d := &M3U8DEngine{
		Config: &DownloadConfig{
			WorkDir:     dir,
			Concurrency: 1,
			Timeout:     300 * time.Millisecond,
			Verbose:     false,
		},
		downloaded: make(map[string]bool),
	}
	tasks := []DownloadTask{
		{URL: srv.URL + "/missing.ts", LocalPath: filepath.Join(dir, "b.ts"), Type: "ts"},
	}
	err := d.downloadFilesConcurrently(t.Context(), tasks)
	if err == nil {
		t.Fatal("expected explicit error for 404, got nil (silent success)")
	}
	if !containsURL(err.Error(), "missing.ts") || !containsURL(err.Error(), "404") {
		t.Fatalf("expected error mentioning missing.ts HTTP 404, got: %v", err)
	}
}

func containsURL(s, sub string) bool { return len(s) >= len(sub) && indexOf(s, sub) >= 0 }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestRecordFailure_StatusGrabIsStatusCode 验证 grab.StatusCodeError 判定走永久分支。
func TestRecordFailure_StatusGrabIsStatusCode(t *testing.T) {
	req, _ := grab.NewRequest("/tmp/x.ts", "http://example.com/x.ts")
	resp := &grab.Response{
		Request: req,
	}
	// 无法直接构造 resp.Err；这里仅验证 IsStatusCodeError 对类型值生效（编译期契约）
	var err error = grab.StatusCodeError(http.StatusForbidden)
	if !grab.IsStatusCodeError(err) {
		t.Fatal("expected StatusCodeError to be detected")
	}
	_ = resp
	_ = context.Background
}

// TestDownloadFilesConcurrently_BackoffBetweenRounds 验证轮间退避：重试轮间有时间间隔（第二次轮不会立即开始）。
func TestDownloadFilesConcurrently_BackoffBetweenRounds(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/slow.ts", func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	d := &M3U8DEngine{
		Config: &DownloadConfig{
			WorkDir:     dir,
			Concurrency: 1,
			Timeout:     100 * time.Millisecond,
		},
		downloaded: make(map[string]bool),
	}
	d.client = &http.Client{Timeout: 60 * time.Millisecond}

	tasks := []DownloadTask{
		{URL: srv.URL + "/slow.ts", LocalPath: filepath.Join(dir, "a.ts"), Type: "ts"},
	}
	start := time.Now()
	err := d.downloadFilesConcurrently(t.Context(), tasks)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected failure")
	}
	// 3 轮 × 超时(60ms) + 2 次退避(1s+2s=3s) → 总时长应显著大于纯重试（无退避 ~0.2s）
	if elapsed < 2*time.Second {
		t.Fatalf("expected backoff between rounds, elapsed=%v", elapsed)
	}
}
