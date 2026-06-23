// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

// TestTkDetector verifies that a ResponseCheck detecting "tk" URLs
// with suspicious Content-Length correctly rejects the download.
func TestTkDetector(t *testing.T) {
	// Simulate a "tk" URL that returns Content-Length 146 without MD5
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", "146")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("x")) // won't get to write 鈥?ErrNoTry before copy
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "tk_out.bin")

	ext := newHTTPExtractor(t)
	// Inject tk detector
	ext.AddResponseCheck(func(req *download.Request, tresp *download.TransportResponse) error {
		// Heuristic: "tk" in URL + no MD5 + Content-Length=146 or -1
		if contains(req.URL, "tk") {
			wantMd5 := download.TryGetMd5(tresp.Headers)
			if wantMd5 == "" && (tresp.ContentLength == 146 || tresp.ContentLength == -1) {
				return fmt.Errorf("%w: suspicious tk URL content length: %d", download.ErrNoTry, tresp.ContentLength)
			}
		}
		return nil
	})

	err := ext.Extract(t.Context(), &download.Request{
		URL:      ts.URL + "?tk=video123", // URL contains "tk"
		SavePath: dest,
	})
	if !download.IsNoTry(err) {
		t.Fatalf("expected ErrNoTry for tk URL with suspicious content length, got: %v", err)
	}
	t.Logf("tk detection correctly rejected: %v", err)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestTkDetectorNormal verifies that a "tk" URL with valid content passes the check.
func TestTkDetectorNormal(t *testing.T) {
	content := "normal-content-with-valid-length"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "tk_normal.bin")

	ext := newHTTPExtractor(t)
	ext.AddResponseCheck(func(req *download.Request, tresp *download.TransportResponse) error {
		if contains(req.URL, "tk") {
			wantMd5 := download.TryGetMd5(tresp.Headers)
			if wantMd5 == "" && (tresp.ContentLength == 146 || tresp.ContentLength == -1) {
				return fmt.Errorf("%w: suspicious tk URL content length: %d", download.ErrNoTry, tresp.ContentLength)
			}
		}
		return nil
	})

	err := ext.Extract(t.Context(), &download.Request{
		URL:      ts.URL + "?tk=video123",
		SavePath: dest,
	})
	if err != nil {
		t.Fatalf("expected success for normal tk URL, got: %v", err)
	}
}
