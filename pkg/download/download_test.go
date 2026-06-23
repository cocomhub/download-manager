// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

// ---- compile-time interface checks ----

type mockExtractor struct{}

func (m *mockExtractor) Name() string                                         { return "mock" }
func (m *mockExtractor) Match(_ context.Context, _ string) bool               { return true }
func (m *mockExtractor) Extract(_ context.Context, _ *download.Request) error { return nil }

type mockTransport struct{}

func (m *mockTransport) Name() string { return "mock" }
func (m *mockTransport) RoundTrip(_ context.Context, _ *download.TransportRequest) (*download.TransportResponse, error) {
	return nil, nil
}

type mockProxySelector struct{}

func (m *mockProxySelector) Select(_ context.Context, _ string, _ *download.DownloadHint) (string, error) {
	return "", nil
}

type mockSelector struct{}

func (m *mockSelector) MatchExtractor(_ context.Context, _ string, _ *download.DownloadHint) download.Extractor {
	return nil
}
func (m *mockSelector) SelectProxy(_ context.Context, _ string, _ *download.DownloadHint) (string, error) {
	return "", nil
}

var (
	_ download.Extractor     = (*mockExtractor)(nil)
	_ download.Transport     = (*mockTransport)(nil)
	_ download.ProxySelector = (*mockProxySelector)(nil)
	_ download.Selector      = (*mockSelector)(nil)
)

// ---- ErrNoTry ----

func TestErrNoTry(t *testing.T) {
	if !download.IsNoTry(download.ErrNoTry) {
		t.Error("IsNoTry(ErrNoTry) should be true")
	}
	if download.IsNoTry(io.EOF) {
		t.Error("IsNoTry(io.EOF) should be false")
	}
}

// ---- Request ----

func TestRequestStruct(t *testing.T) {
	progressCalled := false
	req := &download.Request{
		URL:           "https://example.com/file.zip",
		SavePath:      "/tmp/file.zip",
		Headers:       map[string]string{"Authorization": "Bearer token"},
		TrackProgress: true,
		OnProgress: func(progress float64, downloaded, total int64) {
			progressCalled = true
		},
		Metadata: map[string]string{"key": "val"},
		Hint:     nil,
	}
	if req.URL != "https://example.com/file.zip" {
		t.Errorf("unexpected URL: %s", req.URL)
	}
	if req.SavePath != "/tmp/file.zip" {
		t.Errorf("unexpected SavePath: %s", req.SavePath)
	}
	if req.Headers["Authorization"] != "Bearer token" {
		t.Errorf("unexpected Header")
	}
	if !req.TrackProgress {
		t.Error("TrackProgress should be true")
	}
	req.OnProgress(50, 500, 1000)
	if !progressCalled {
		t.Error("OnProgress should have been called")
	}
	if req.Metadata["key"] != "val" {
		t.Errorf("unexpected Metadata")
	}
}

func TestRequestProgressCallback(t *testing.T) {
	var p float64
	var d int64
	req := &download.Request{
		URL:           "https://example.com/bigfile.bin",
		TrackProgress: true,
		OnProgress: func(progress float64, downloaded, _ int64) {
			p = progress
			d = downloaded
		},
	}
	req.OnProgress(0.5, 512, 1024)
	if p != 0.5 {
		t.Errorf("expected progress 0.5, got %f", p)
	}
	if d != 512 {
		t.Errorf("expected downloaded 512, got %d", d)
	}
}

// ---- DownloadHint ----

func TestDownloadHintStruct(t *testing.T) {
	hint := &download.DownloadHint{
		FileSize:    1024,
		ContentType: "application/zip",
		Extractor:   "native",
		Tags:        map[string]string{"quality": "high"},
	}
	if hint.FileSize != 1024 {
		t.Errorf("unexpected FileSize: %d", hint.FileSize)
	}
	if hint.ContentType != "application/zip" {
		t.Errorf("unexpected ContentType: %s", hint.ContentType)
	}
	if hint.Extractor != "native" {
		t.Errorf("unexpected Extractor: %s", hint.Extractor)
	}
	if hint.Tags["quality"] != "high" {
		t.Errorf("unexpected Tag")
	}
}

// ---- TransportRequest / TransportResponse ----

func TestTransportRequestStruct(t *testing.T) {
	treq := &download.TransportRequest{
		URL:      "https://cdn.example.com/file.bin",
		Method:   "GET",
		Headers:  map[string]string{"Range": "bytes=0-1023"},
		Body:     []byte("hello"),
		Range:    &download.RangeRequest{Offset: 0},
		ProxyURL: "http://proxy:8080",
	}
	if treq.URL != "https://cdn.example.com/file.bin" {
		t.Errorf("unexpected URL")
	}
	if treq.Method != "GET" {
		t.Errorf("unexpected Method")
	}
	if string(treq.Body) != "hello" {
		t.Errorf("unexpected Body")
	}
	if treq.ProxyURL != "http://proxy:8080" {
		t.Errorf("unexpected ProxyURL")
	}
}

func TestTransportResponseStruct(t *testing.T) {
	body := io.NopCloser(strings.NewReader("response body"))
	tresp := &download.TransportResponse{
		Body:          body,
		StatusCode:    200,
		ContentLength: 13,
		Headers:       map[string]string{"Content-Type": "application/octet-stream"},
		ProxyURL:      "http://proxy:8080",
	}
	if tresp.StatusCode != 200 {
		t.Errorf("unexpected StatusCode: %d", tresp.StatusCode)
	}
	if tresp.ContentLength != 13 {
		t.Errorf("unexpected ContentLength: %d", tresp.ContentLength)
	}
	if tresp.Headers["Content-Type"] != "application/octet-stream" {
		t.Errorf("unexpected Content-Type header")
	}
	// verify body is readable
	data, err := io.ReadAll(tresp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if string(data) != "response body" {
		t.Errorf("unexpected body content: %s", string(data))
	}
	tresp.Body.Close()
}

// ---- RangeRequest ----

func TestRangeRequestStruct(t *testing.T) {
	rr := &download.RangeRequest{Offset: 1024}
	if rr.Offset != 1024 {
		t.Errorf("unexpected Offset: %d", rr.Offset)
	}
}

// ---- Extractor interface mock usage ----

func TestExtractorInterface(t *testing.T) {
	e := &mockExtractor{}
	if e.Name() != "mock" {
		t.Errorf("unexpected name: %s", e.Name())
	}
	if !e.Match(t.Context(), "http://example.com") {
		t.Error("Match should return true")
	}
	if err := e.Extract(t.Context(), &download.Request{URL: "http://example.com"}); err != nil {
		t.Errorf("Extract should not error: %v", err)
	}
}

// ---- Transport interface mock usage ----

func TestTransportInterface(t *testing.T) {
	tr := &mockTransport{}
	if tr.Name() != "mock" {
		t.Errorf("unexpected name: %s", tr.Name())
	}
	resp, err := tr.RoundTrip(t.Context(), &download.TransportRequest{URL: "http://example.com"})
	if err != nil {
		t.Errorf("RoundTrip should not error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %+v", resp)
	}
}

// ---- ProxySelector interface mock usage ----

func TestProxySelectorInterface(t *testing.T) {
	ps := &mockProxySelector{}
	proxy, err := ps.Select(t.Context(), "http://example.com", nil)
	if err != nil {
		t.Errorf("Select should not error: %v", err)
	}
	if proxy != "" {
		t.Errorf("expected empty proxy, got %s", proxy)
	}
}

// ---- Selector interface mock usage ----

func TestSelectorInterface(t *testing.T) {
	s := &mockSelector{}
	ext := s.MatchExtractor(t.Context(), "http://example.com", nil)
	if ext != nil {
		t.Errorf("expected nil extractor, got %+v", ext)
	}
	proxy, err := s.SelectProxy(t.Context(), "http://example.com", nil)
	if err != nil {
		t.Errorf("SelectProxy should not error: %v", err)
	}
	if proxy != "" {
		t.Errorf("expected empty proxy, got %s", proxy)
	}
}

// ---- Edge cases ----

func TestRequestNilOnProgress(t *testing.T) {
	req := &download.Request{
		URL:           "http://example.com/file",
		TrackProgress: true,
		OnProgress:    nil,
	}
	// Should not panic when OnProgress is nil
	if req.OnProgress != nil {
		t.Error("OnProgress should be nil")
	}
}

func TestTransportRequestNilRange(t *testing.T) {
	treq := &download.TransportRequest{
		URL:    "http://example.com/file",
		Method: "GET",
		Range:  nil,
	}
	if treq.Range != nil {
		t.Error("Range should be nil")
	}
}

func TestTransportResponseNilBody(t *testing.T) {
	tresp := &download.TransportResponse{
		Body:          nil,
		StatusCode:    404,
		ContentLength: 0,
	}
	if tresp.Body != nil {
		t.Error("Body should be nil")
	}
	if tresp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", tresp.StatusCode)
	}
}

func TestDownloadHintNilTags(t *testing.T) {
	hint := &download.DownloadHint{
		FileSize:    0,
		ContentType: "",
		Extractor:   "",
		Tags:        nil,
	}
	if hint.Tags != nil {
		t.Error("Tags should be nil")
	}
}

func TestErrNoTryWrapping(t *testing.T) {
	err := download.ErrNoTry
	if !errors.Is(err, download.ErrNoTry) {
		t.Error("errors.Is(ErrNoTry, ErrNoTry) should be true")
	}
	wrapped := fmt.Errorf("download failed after retries: %w", download.ErrNoTry)
	if !errors.Is(wrapped, download.ErrNoTry) {
		t.Error("errors.Is(wrapped, ErrNoTry) should be true")
	}
	if !download.IsNoTry(wrapped) {
		t.Error("IsNoTry(wrapped) should be true")
	}
}

// ---- Downloader tests ----

func TestNewDownloader(t *testing.T) {
	d := download.New()
	if d == nil {
		t.Fatal("New() should not return nil")
	}
}

func TestDownloaderInvalidRequestNil(t *testing.T) {
	d := download.New()
	err := d.Download(t.Context(), nil)
	if err == nil {
		t.Error("Download with nil request should error")
	}
}

func TestDownloaderInvalidRequestEmptyURL(t *testing.T) {
	d := download.New()
	err := d.Download(t.Context(), &download.Request{SavePath: "/tmp/file"})
	if err == nil {
		t.Error("Download with empty URL should error")
	}
}

func TestDownloaderInvalidRequestEmptySavePath(t *testing.T) {
	d := download.New()
	err := d.Download(t.Context(), &download.Request{URL: "http://example.com"})
	if err == nil {
		t.Error("Download with empty SavePath should error")
	}
}

func TestDownloaderNoExtractor(t *testing.T) {
	d := download.New()
	err := d.Download(t.Context(), &download.Request{
		URL:      "http://example.com/file",
		SavePath: "/tmp/file",
	})
	if err == nil {
		t.Error("Download with no extractor should error")
	}
}

func TestDownloaderWithExtractor(t *testing.T) {
	ex := &mockExtractor{}
	sel := download.NewDefaultSelector()
	d := download.New(download.WithExtractor(ex), download.WithSelector(sel))
	err := d.Download(t.Context(), &download.Request{
		URL:      "http://example.com/file",
		SavePath: "/tmp/file",
	})
	if err != nil {
		t.Errorf("Download with extractor should not error: %v", err)
	}
}

func TestDownloaderWithSelector(t *testing.T) {
	sel := &mockSelector{}
	d := download.New(download.WithSelector(sel))
	if d == nil {
		t.Fatal("New() with selector should not return nil")
	}
}

func TestDownloaderWithTransport(t *testing.T) {
	tr := &mockTransport{}
	d := download.New(download.WithTransport(tr))
	if d == nil {
		t.Fatal("New() with transport should not return nil")
	}
}

// ---- Global Get/Default/SetDefault tests ----

func TestDefaultNilBeforeSet(t *testing.T) {
	// Default() should return a valid default (lazy init)
	dl := download.Default()
	if dl == nil {
		t.Error("Default() should return a valid downloader after lazy init")
	}
}

func TestGetReturnsNoError(t *testing.T) {
	// Get() now lazy-initializes, so it shouldn't return ErrNoDefaultDownloader
	err := download.Get(t.Context(), "http://example.com/file", "/tmp/file")
	if err == download.ErrNoDefaultDownloader {
		t.Errorf("Get() should not return ErrNoDefaultDownloader after lazy init")
	}
}

func TestSetDefaultAndGet(t *testing.T) {
	ex := &mockExtractor{}
	sel := download.NewDefaultSelector()
	d := download.New(download.WithExtractor(ex), download.WithSelector(sel))
	download.SetDefault(d)
	t.Cleanup(func() { download.SetDefault(nil) })

	if download.Default() != d {
		t.Error("Default() should return the set downloader")
	}

	err := download.Get(t.Context(), "http://example.com/file", "/tmp/file")
	if err != nil {
		t.Errorf("Get() should not error: %v", err)
	}
}

// ---- Integration test with real server ----

func TestDownloaderRealDownload(t *testing.T) {
	// Start a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer ts.Close()

	// Create a temp dir
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "output.txt")

	// Create downloader with HTTP extractor and stdlib transport
	ex := download.NewHTTPExtractor()
	tr := download.NewStdlibTransport()
	d := download.New(download.WithExtractor(ex), download.WithTransport(tr))

	err := d.Download(t.Context(), &download.Request{
		URL:      ts.URL,
		SavePath: savePath,
	})
	if err != nil {
		t.Fatalf("Download should succeed: %v", err)
	}

	// Verify file content
	data, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("unexpected content: got %q, want %q", string(data), "hello world")
	}
}

func TestDownloaderWithHintExtractor(t *testing.T) {
	// When hint specifies an extractor name, DefaultSelector should use it
	ex := &mockExtractor{}
	sel := download.NewDefaultSelector()
	d := download.New(
		download.WithExtractor(ex),
		download.WithSelector(sel),
	)

	// The mockExtractor.Match returns false, but hint-based match works by name
	err := d.Download(t.Context(), &download.Request{
		URL:      "http://example.com/file",
		SavePath: "/tmp/file",
		Hint:     &download.DownloadHint{Extractor: "mock"},
	})
	if err != nil {
		t.Errorf("should not error but got: %v", err)
	}
}
