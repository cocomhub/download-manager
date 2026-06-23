// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

// TestHTTPExtractorContentTypeWithQuery verifies that Content-Type validation
// correctly extracts the file extension from the URL path, ignoring query parameters.
func TestHTTPExtractorContentTypeWithQuery(t *testing.T) {
	// Server returns text/html for a .mp4 URL 鈥?should trigger Content-Type check
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>not a video</html>"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "output.bin")

	ext := newHTTPExtractor(t)
	req := &download.Request{
		// URL has .mp4 extension but with query params.
		// Current bug: filepath.Ext(rawURL) returns ".mp4?token=abc"
		// which doesn't match mediaExtensionSet[".mp4"], so Content-Type check is SKIPPED.
		// After fix: should correctly detect ".mp4" from parsed.Path and reject text/html.
		URL:      ts.URL + "/video.mp4?token=abc&expires=123",
		SavePath: dest,
	}
	err := ext.Extract(t.Context(), req)
	if !download.IsNoTry(err) {
		t.Fatalf("expected ErrNoTry for text/html with .mp4 URL (with query params), got: %v (bug: filepath.Ext parses raw URL including query string, check was bypassed)", err)
	}
}
