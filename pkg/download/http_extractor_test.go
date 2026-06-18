// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

func TestHTTPExtractorCaching(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "304 skip",
			run: func(t *testing.T) {
				var capturedEtag string
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if inm := r.Header.Get("If-None-Match"); inm != "" {
						w.Header().Set("ETag", `"abc123"`)
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

				// First download: normal download, saves ETag, OnMetadata fires.
				ext := download.NewHTTPExtractor()
				ext.SetTransport(download.NewStdlibTransport())
				req := &download.Request{
					URL:      ts.URL,
					SavePath: dest,
					Metadata: map[string]string{},
					OnMetadata: func(key, value string) {
						if key == "etag" {
							capturedEtag = value
						}
					},
				}
				err := ext.Extract(t.Context(), req)
				if err != nil {
					t.Fatalf("first download failed: %v", err)
				}
				data, _ := os.ReadFile(dest)
				if string(data) != "hello world" {
					t.Errorf("expected 'hello world', got '%s'", data)
				}
				if capturedEtag != `"abc123"` {
					t.Errorf("expected OnMetadata etag 'abc123', got '%s'", capturedEtag)
				}
				if req.Metadata["etag"] != `"abc123"` {
					t.Errorf("expected metadata etag 'abc123', got '%s'", req.Metadata["etag"])
				}

				// Second download: carries ETag, verifies 304 OnMetadata fires.
				capturedEtag = ""
				req2 := &download.Request{
					URL:      ts.URL,
					SavePath: dest,
					Metadata: map[string]string{"etag": `"abc123"`},
					OnMetadata: func(key, value string) {
						if key == "etag" {
							capturedEtag = value
						}
					},
				}
				err = ext.Extract(t.Context(), req2)
				if err != nil {
					t.Fatalf("second download (304) failed: %v", err)
				}
				data2, _ := os.ReadFile(dest)
				if string(data2) != "hello world" {
					t.Errorf("expected file content unchanged 'hello world', got '%s'", data2)
				}
				if req2.Result == nil || req2.Result.StatusCode != http.StatusNotModified {
					code := 0
					if req2.Result != nil {
						code = req2.Result.StatusCode
					}
					t.Errorf("expected StatusCode=304, got %d", code)
				}
				if capturedEtag != `"abc123"` {
					t.Errorf("expected OnMetadata etag from 304 '%s', got '%s'", `"abc123"`, capturedEtag)
				}
			},
		},
		{
			name: "If-None-Match header",
			run: func(t *testing.T) {
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
				_ = ext.Extract(t.Context(), req)
				if seenHeader != `"xyz789"` {
					t.Errorf("expected If-None-Match header 'xyz789', got '%s'", seenHeader)
				}
			},
		},
		{
			name: "no prior ETag",
			run: func(t *testing.T) {
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
					Metadata: map[string]string{},
				}
				_ = ext.Extract(t.Context(), req)
				if seenETagHeader {
					t.Error("If-None-Match should not be sent without prior ETag")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, tc.run)
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
	err := ext.Extract(t.Context(), req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected content 'hello world', got: %q", string(data))
	}

	if req.Result.StatusCode != 200 {
		t.Errorf("expected Result.StatusCode=200, got: %d", req.Result.StatusCode)
	}
	if req.Result.ContentLength <= 0 {
		t.Errorf("expected Result.ContentLength to be set, got: %d", req.Result.ContentLength)
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
	err := ext.Extract(t.Context(), &download.Request{
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

func TestHTTPExtractorResume(t *testing.T) {
	tests := []struct {
		name string
		// serverHandler returns a handler that may or may not support resume.
		serverHandler func(t *testing.T) http.HandlerFunc
		// expectedContent is the full file content after download completes.
		expectedContent string
		// initialContent is written to the destination before Extract.
		initialContent string
	}{
		{
			name: "supported",
			serverHandler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Range") == "" {
						t.Error("expected Range header for resume download")
					}
					w.WriteHeader(http.StatusPartialContent)
					_, _ = w.Write([]byte("resumed_content"))
				}
			},
			initialContent:  "partial_",
			expectedContent: "partial_resumed_content",
		},
		{
			name: "unsupported",
			serverHandler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					// Always return 200 even when client requests Range.
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("full_content"))
				}
			},
			initialContent:  "part_",
			expectedContent: "full_content",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, "resume.txt")

			if err := os.WriteFile(dest, []byte(tc.initialContent), 0644); err != nil {
				t.Fatal(err)
			}

			ts := httptest.NewServer(tc.serverHandler(t))
			defer ts.Close()

			ext := download.NewHTTPExtractor()
			ext.SetTransport(download.NewStdlibTransport())

			err := ext.Extract(t.Context(), &download.Request{
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
			if string(data) != tc.expectedContent {
				t.Errorf("expected %q, got: %q", tc.expectedContent, string(data))
			}
		})
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

	err := ext.Extract(t.Context(), &download.Request{
		URL:      ts.URL,
		SavePath: dest,
	})
	if !download.IsNoTry(err) {
		t.Fatalf("expected ErrNoTry for 404, got: %v", err)
	}
}

func TestHTTPExtractorRetriesOnError(t *testing.T) {
	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
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

	err := ext.Extract(t.Context(), &download.Request{
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
	if ext.Match(t.Context(), "http://example.com/video.m3u8") {
		t.Error("expected Match to return false for .m3u8 URLs")
	}
	if !ext.Match(t.Context(), "http://example.com/video.mp4") {
		t.Error("expected Match to return true for non-m3u8 URLs")
	}
}

func TestHTTPExtractorName(t *testing.T) {
	ext := download.NewHTTPExtractor()
	if ext.Name() != "http" {
		t.Errorf("expected name 'http', got: %q", ext.Name())
	}
}

// TestHTTPExtractorOnMetadataFires verifies OnMetadata fires all keys after a successful download.
func TestHTTPExtractorOnMetadataFires(t *testing.T) {
	var (
		mu             sync.Mutex
		seenEtag       bool
		seenChecksum   bool
		serverChecksum string
		serverEtag     string
	)

	content := "test file content for md5"
	h := md5.New()
	io.WriteString(h, content)
	hexMD5 := hex.EncodeToString(h.Sum(nil))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"my-file-etag"`)
		w.Header().Set("Content-MD5", hexMD5)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "on_metadata.txt")

	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())
	req := &download.Request{
		URL:      ts.URL,
		SavePath: dest,
		Metadata: map[string]string{},
		OnMetadata: func(key, value string) {
			mu.Lock()
			defer mu.Unlock()
			switch key {
			case "etag":
				seenEtag = true
				serverEtag = value
			case "checksum":
				seenChecksum = true
				serverChecksum = value
			}
		},
	}
	err := ext.Extract(t.Context(), req)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	mu.Lock()
	if !seenEtag {
		t.Error("OnMetadata did not fire for 'etag'")
	}
	if !seenChecksum {
		t.Error("OnMetadata did not fire for 'checksum'")
	}
	if serverEtag != `"my-file-etag"` {
		t.Errorf("expected etag '\"my-file-etag\"', got '%s'", serverEtag)
	}
	if serverChecksum != hexMD5 {
		t.Errorf("expected checksum '%s', got '%s'", hexMD5, serverChecksum)
	}
	mu.Unlock()

	if req.Metadata["etag"] != `"my-file-etag"` {
		t.Errorf("metadata etag: expected '\"my-file-etag\"', got '%s'", req.Metadata["etag"])
	}
	if req.Metadata["checksum"] != hexMD5 {
		t.Errorf("metadata checksum: expected '%s', got '%s'", hexMD5, req.Metadata["checksum"])
	}
}

// TestHTTPExtractorNoEtagFallback verifies OnMetadata synthesizes a weak ETag from Content-MD5
// when the server doesn't provide an ETag header.
func TestHTTPExtractorNoEtagFallback(t *testing.T) {
	content := "some fallback content"
	h := md5.New()
	io.WriteString(h, content)
	hexMD5 := hex.EncodeToString(h.Sum(nil))

	var capturedEtag string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-MD5", hexMD5)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "no_etag_file.txt")

	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())

	req := &download.Request{
		URL:      ts.URL,
		SavePath: dest,
		Metadata: map[string]string{},
		OnMetadata: func(key, value string) {
			if key == "etag" {
				capturedEtag = value
			}
		},
	}
	err := ext.Extract(t.Context(), req)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	wantEtag := `"` + hexMD5 + `"`
	if capturedEtag != wantEtag {
		t.Errorf("expected fallback etag '%s', got '%s'", wantEtag, capturedEtag)
	}
}
