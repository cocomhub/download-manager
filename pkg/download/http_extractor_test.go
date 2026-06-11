// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

func TestHTTPExtractorETagSkip(t *testing.T) {
	// 测试：服务器返回 304 时，跳过下载
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if inm := r.Header.Get("If-None-Match"); inm != "" {
			// 检测是否有 etag 在 Metadata 中，如果有说明是第二次
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "etag_skip.txt")

	// 第一次下载：正常下载，保存 ETag
	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())
	req := &download.Request{
		URL:      ts.URL,
		SavePath: dest,
		Metadata: map[string]string{},
	}
	err := ext.Extract(context.Background(), req)
	if err != nil {
		t.Fatalf("first download failed: %v", err)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", data)
	}
	if req.Metadata["etag"] != `"abc123"` {
		t.Errorf("expected etag 'abc123', got '%s'", req.Metadata["etag"])
	}

	// 第二次下载：携带 ETag（不含 checksum，模拟旧数据场景）
	// 代码应发送 If-None-Match，服务器返回 304，文件内容保持
	req2 := &download.Request{
		URL:      ts.URL,
		SavePath: dest,
		Metadata: map[string]string{"etag": `"abc123"`},
	}
	err = ext.Extract(context.Background(), req2)
	if err != nil {
		t.Fatalf("second download (304) failed: %v", err)
	}
	// 文件应保持原内容不变
	data2, _ := os.ReadFile(dest)
	if string(data2) != "hello world" {
		t.Errorf("expected file content unchanged 'hello world', got '%s'", data2)
	}
	// 304 场景下 Result.StatusCode 应有指示
	if req2.Result == nil || req2.Result.StatusCode != http.StatusNotModified {
		t.Errorf("expected StatusCode=304, got %v", func() int {
			if req2.Result == nil {
				return 0
			}
			return req2.Result.StatusCode
		}())
	}
}

func TestHTTPExtractorIfNoneMatch(t *testing.T) {
	// 测试：If-None-Match 头是否正确发送
	var seenHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHeader = r.Header.Get("If-None-Match")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())
	req := &download.Request{
		URL:      ts.URL,
		SavePath: filepath.Join(dir, "inm.txt"),
		Metadata: map[string]string{"etag": `"xyz789"`},
	}
	_ = ext.Extract(context.Background(), req)
	if seenHeader != `"xyz789"` {
		t.Errorf("expected If-None-Match header 'xyz789', got '%s'", seenHeader)
	}
}

func TestHTTPExtractorNoETag(t *testing.T) {
	// 测试：无 ETag 元数据时不发送 If-None-Match
	var seenETagHeader bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" {
			seenETagHeader = true
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())
	req := &download.Request{
		URL:      ts.URL,
		SavePath: filepath.Join(dir, "noetag.txt"),
		Metadata: map[string]string{}, // 无 etag
	}
	_ = ext.Extract(context.Background(), req)
	if seenETagHeader {
		t.Error("If-None-Match should not be sent without prior ETag")
	}
}

func TestHTTPExtractorBasic(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer ts.Close()
	dir := t.TempDir()
	dest := filepath.Join(dir, "output.txt")

	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())

	req := &download.Request{
		URL:           ts.URL,
		SavePath:      dest,
		TrackProgress: false,
		Metadata:      make(map[string]string),
	}
	err := ext.Extract(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// 验证文件内容
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected content 'hello world', got: %q", string(data))
	}

	// 验证 DownloadResult
	if req.Result.StatusCode != 200 {
		t.Errorf("expected Result.StatusCode=200, got: %d", req.Result.StatusCode)
	}
	if req.Result.ContentLength <= 0 {
		t.Errorf("expected Result.ContentLength to be set, got: %d", req.Result.ContentLength)
	}
}

func TestHTTPExtractor404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	dir := t.TempDir()
	dest := filepath.Join(dir, "output.txt")

	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())

	err := ext.Extract(context.Background(), &download.Request{
		URL:      ts.URL,
		SavePath: dest,
	})
	if !download.IsNoTry(err) {
		t.Fatalf("expected ErrNoTry for 404, got: %v", err)
	}
}

func TestHTTPExtractorWithProgress(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world progress test payload"))
	}))
	defer ts.Close()
	dir := t.TempDir()
	dest := filepath.Join(dir, "output.txt")

	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())

	var finalProgress float64
	err := ext.Extract(context.Background(), &download.Request{
		URL:           ts.URL,
		SavePath:      dest,
		TrackProgress: true,
		OnProgress: func(progress float64, downloaded, total int64) {
			finalProgress = progress
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if finalProgress != 100 {
		t.Errorf("expected final progress 100, got: %f", finalProgress)
	}
}

func TestHTTPExtractorWithResume(t *testing.T) {
	// 验证断点续传：先写部分内容到文件，然后下载应追加
	dir := t.TempDir()
	dest := filepath.Join(dir, "resume.txt")

	// 先写入部分内容模拟已下载部分
	if err := os.WriteFile(dest, []byte("partial_"), 0644); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求头携带了 Range
		if r.Header.Get("Range") == "" {
			t.Error("expected Range header for resume download")
		}
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("resumed_content"))
	}))
	defer ts.Close()

	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())

	err := ext.Extract(context.Background(), &download.Request{
		URL:      ts.URL,
		SavePath: dest,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	expected := "partial_resumed_content"
	if string(data) != expected {
		t.Errorf("expected %q, got: %q", expected, string(data))
	}
}

func TestHTTPExtractorRetriesOnError(t *testing.T) {
	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			// 前两次返回 500
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success after retry"))
	}))
	defer ts.Close()
	dir := t.TempDir()
	dest := filepath.Join(dir, "retry.txt")

	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())

	err := ext.Extract(context.Background(), &download.Request{
		URL:      ts.URL,
		SavePath: dest,
	})
	if err != nil {
		t.Fatalf("expected no error after retry, got: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "success after retry" {
		t.Errorf("expected 'success after retry', got: %q", string(data))
	}
}

func TestHTTPExtractorMatchRejectsM3U8(t *testing.T) {
	ext := download.NewHTTPExtractor()
	if ext.Match(context.Background(), "http://example.com/video.m3u8") {
		t.Error("expected Match to return false for .m3u8 URLs")
	}
	if !ext.Match(context.Background(), "http://example.com/video.mp4") {
		t.Error("expected Match to return true for non-m3u8 URLs")
	}
}

func TestHTTPExtractorName(t *testing.T) {
	ext := download.NewHTTPExtractor()
	if ext.Name() != "http" {
		t.Errorf("expected name 'http', got: %q", ext.Name())
	}
}

func TestHTTPExtractorResumeUnsupported(t *testing.T) {
	// 验证服务器不支持断点续传（返回 200 而非 206）时，
	// 文件最终内容完整不重复
	dir := t.TempDir()
	dest := filepath.Join(dir, "resume_unsupported.txt")

	// 先写入部分内容模拟已下载部分
	if err := os.WriteFile(dest, []byte("part_"), 0644); err != nil {
		t.Fatal(err)
	}

	var sawRange bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			sawRange = true
		}
		// 始终返回 200 ，即使用户请求了 Range
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("full_content"))
	}))
	defer ts.Close()

	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())

	err := ext.Extract(context.Background(), &download.Request{
		URL:      ts.URL,
		SavePath: dest,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// 验证服务器确实接收到了 Range 请求（测试本身的有效性）
	if !sawRange {
		t.Error("expected server to receive Range header")
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	// 文件应为完整内容，而非 "part_full_content"（即非部分+完整内容拼接）
	expected := "full_content"
	if string(data) != expected {
		t.Errorf("expected %q (complete content), got: %q", expected, string(data))
	}
}
