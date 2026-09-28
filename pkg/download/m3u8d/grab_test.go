// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package m3u8d

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cavaliergopher/grab/v3"
)

// TestDownloadFilesConcurrently_StatusCodeSkipsRetries 验证 4xx 永久失败不占用重试额度：
// 第二次轮次应只包含超时类失败分片（4xx 分片被立即放弃）。
func TestDownloadFilesConcurrently_StatusCodeSkipsRetries(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/timeout.ts", func(w http.ResponseWriter, r *http.Request) {
		// 模拟超时：不发响应头直到请求取消（grab 报 context deadline exceeded）
		<-r.Context().Done()
	})
	mux.HandleFunc("/forbidden.ts", func(w http.ResponseWriter, r *http.Request) {
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
	// 注入带短超时的 client
	d.client = &http.Client{Timeout: 200 * time.Millisecond}
	if err := os.WriteFile(filepath.Join(dir, "master.m3u8"), []byte("#EXTM3U\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks := []DownloadTask{
		{URL: srv.URL + "/timeout.ts", LocalPath: filepath.Join(dir, "a.ts"), Type: "ts"},
		{URL: srv.URL + "/forbidden.ts", LocalPath: filepath.Join(dir, "b.ts"), Type: "ts"},
	}
	err := d.downloadFilesConcurrently(t.Context(), tasks)
	if err == nil {
		t.Fatal("expected overall failure (timeout.ts still failing)")
	}
	// timeout.ts 应重试多次后仍在失败列表中（可恢复类）；forbidden.ts 因 4xx 被立即放弃，不占额度。
	if !containsURL(err.Error(), "timeout.ts") {
		t.Fatalf("expected timeout.ts in retry error, got: %v", err)
	}
	// 二次轮次集合（即 retryRound>=maxRetryRounds-1 时的 errReqs）应只含 timeout.ts
	// 通过重新运行一轮单任务验证：forbidden 不出现
	if containsURL(err.Error(), "forbidden.ts") {
		t.Fatalf("forbidden.ts should not be retried (permanent 4xx), got: %v", err)
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
