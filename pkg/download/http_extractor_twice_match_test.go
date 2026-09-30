// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

// TestHTTPExtractor_NoComparableMD5_StableContentPasses 验证 ETag 非 MD5 形态（无法比较）
// 但内容稳定时：重复下载两次内容一致后通过（不报错，文件保留最后一次内容）。
func TestHTTPExtractor_NoComparableMD5_StableContentPasses(t *testing.T) {
	const body = "fourhoi-stable-cover-content"
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("ETag", `"opaque-v1"`) // 非 MD5 形态
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	root := t.TempDir()
	ex := download.NewHTTPExtractorWithConfig(5, "TestUA", root, "")
	ex.SetTransport(download.NewStdlibTransport())
	req := &download.Request{
		URL:      srv.URL + "/cover.jpg",
		SavePath: root + "/c.jpg",
	}
	if err := ex.Extract(t.Context(), req); err != nil {
		t.Fatalf("stable content with non-MD5 ETag should pass via two identical downloads: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("expected exactly 2 downloads (two identical to accept), got %d", got)
	}
	data, err := os.ReadFile(req.SavePath)
	if err != nil || string(data) != body {
		t.Errorf("unexpected final content: err=%v data=%q", err, data)
	}
	if req.Result == nil || req.Result.MD5Hex == "" {
		t.Error("expected checksum recorded in result")
	}
}

// TestHTTPExtractor_NoComparableMD5_ChangingContentFails 验证 ETag 非 MD5 且每次内容不同时，
// 重试耗尽（两份始终不一致）→ 下载失败。
func TestHTTPExtractor_NoComparableMD5_ChangingContentFails(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("ETag", `"opaque-v1"`) // 非 MD5 形态
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = fmt.Fprintf(w, "changing-content-%d", n)
	}))
	defer srv.Close()

	root := t.TempDir()
	ex := download.NewHTTPExtractorWithConfig(3, "TestUA", root, "") // maxRetries=3
	ex.SetTransport(download.NewStdlibTransport())
	req := &download.Request{
		URL:      srv.URL + "/cover.jpg",
		SavePath: root + "/c.jpg",
	}
	if err := ex.Extract(t.Context(), req); err == nil {
		t.Fatal("changing content with non-MD5 ETag should eventually fail (never two identical)")
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("expected maxRetries=3 attempts = 3 downloads, got %d", got)
	}
}

// TestHTTPExtractor_NoComparableMD5_ReExtractReVerifies 验证每次 Extract 视为一次全新下载：
// 三次同 URL 稳定内容应各需两次下载（共 6 次），不会被上一次的内存 verifyMD5 记录提前接受成单次。
func TestHTTPExtractor_NoComparableMD5_ReExtractReVerifies(t *testing.T) {
	const body = "re-extract-stable-cover"
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("ETag", `"opaque-v1"`) // 非 MD5 形态
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	root := t.TempDir()
	ex := download.NewHTTPExtractorWithConfig(5, "TestUA", root, "")
	ex.SetTransport(download.NewStdlibTransport())
	for range 3 {
		req := &download.Request{URL: srv.URL, SavePath: root + "/c.jpg"}
		if err := ex.Extract(t.Context(), req); err != nil {
			t.Fatalf("extract failed: %v", err)
		}
	}
	if got := calls.Load(); got < 6 {
		t.Errorf("each extract should re-download at least twice (3 x 2 = 6), got %d", got)
	}
}

// TestHTTPExtractor_ComparableMD5_MatchPasses 验证 ETag 为内容 MD5 且匹配 → 首下直通（无回归）。
func TestHTTPExtractor_ComparableMD5_MatchPasses(t *testing.T) {
	body := []byte("etag-md5-match")
	sum := md5.Sum(body)
	hexMD5 := hex.EncodeToString(sum[:])

	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("ETag", `"`+hexMD5+`"`)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	root := t.TempDir()
	ex := download.NewHTTPExtractorWithConfig(5, "TestUA", root, "")
	ex.SetTransport(download.NewStdlibTransport())
	req := &download.Request{URL: srv.URL, SavePath: root + "/f.jpg"}
	if err := ex.Extract(t.Context(), req); err != nil {
		t.Fatalf("matching MD5 should pass on first download: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("expected single download when ETag matches, got %d", got)
	}
}

// TestHTTPExtractor_ComparableMD5_MismatchRetries 验证 ETag 为 MD5 形态但不匹配 →
// 持续重试直至耗尽失败（无回归，现状行为）。
func TestHTTPExtractor_ComparableMD5_MismatchRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"00000000000000000000000000000000"`) // MD5 形态但不匹配
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("mismatch-stable-body"))
	}))
	defer srv.Close()

	ex := download.NewHTTPExtractorWithConfig(1, "TestUA", t.TempDir(), "") // maxRetries=1
	ex.SetTransport(download.NewStdlibTransport())
	req := &download.Request{URL: srv.URL + "/cover.jpg", SavePath: t.TempDir() + "/c.jpg"}
	if err := ex.Extract(t.Context(), req); err == nil {
		t.Fatal("etag=MD5 but mismatch should fail after retries exhausted (no regression)")
	}
}
