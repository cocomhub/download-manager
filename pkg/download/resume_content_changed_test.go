// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

// TestHTTPExtractorResumeContentChanged verifies that when the server-side content
// changes during a resume (Content-Range indicates shorter content than the local file),
// the download resets and downloads the full content from offset 0.
func TestHTTPExtractorResumeContentChanged(t *testing.T) {
	originalContent := "original-long-content-for-testing"
	newContent := "NEW-shorter"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		rh := r.Header.Get("Range")
		if rh != "" {
			// Supports Range but returns content shorter than the existing file
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(newContent)-1, len(newContent)))
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte(newContent))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(newContent))
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "resume_changed.txt")

	// Write a file that is LONGER than what the server's new content
	if err := os.WriteFile(dest, []byte(originalContent), 0644); err != nil {
		t.Fatal(err)
	}

	ext := newHTTPExtractor(t)
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
	// Should have the full new content (either via full download or reset+download)
	if string(data) != newContent {
		t.Errorf("expected full new content %q, got %q (len=%d)", newContent, string(data), len(data))
	}
}
