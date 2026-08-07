// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

// TestIsSensitiveHeader 验证敏感头识别函数对所有已知敏感前缀的覆盖。
func TestIsSensitiveHeader(t *testing.T) {
	tests := []struct {
		name          string
		headerName    string
		wantSensitive bool
	}{
		// 敏感头（必须脱敏）
		{"exact authorization", "authorization", true},
		{"upper Authorization", "Authorization", true},
		{"cookie", "cookie", true},
		{"proxy-authorization", "proxy-authorization", true},
		{"x-api-key", "x-api-key", true},
		{"x-auth-token", "x-auth-token", true},
		{"token", "token", true},
		{"secret-key", "secret-key", true},
		{"auth-bearer", "auth-bearer", true},
		// 非敏感头（不脱敏）
		{"content-type", "content-type", false},
		{"user-agent", "user-agent", false},
		{"accept", "accept", false},
		{"referer", "referer", false},
		{"cache-control", "cache-control", false},
		{"x-request-id", "x-request-id", false},
		{"x-forwarded-for", "x-forwarded-for", false},
		{"x-real-ip", "x-real-ip", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := download.IsSensitiveHeader(tt.headerName)
			if got != tt.wantSensitive {
				t.Errorf("IsSensitiveHeader(%q) = %v, want %v", tt.headerName, got, tt.wantSensitive)
			}
		})
	}
}

// TestHTTPExtractorBrowserHeaders verifies that Chrome-style browser headers are injected.
func TestHTTPExtractorBrowserHeaders(t *testing.T) {
	var capturedHeaders http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")

	ext := newHTTPExtractor(t)
	ext.SetBrowserHeaders(true)
	req := &download.Request{
		URL:      ts.URL,
		SavePath: dest,
	}
	err := ext.Extract(t.Context(), req)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	// Check key browser headers
	type headerCheck struct {
		name   string
		key    string
		expect string
	}
	checks := []headerCheck{
		{"sec-ch-ua", "Sec-Ch-Ua", `"Google Chrome";v="145", "Chromium";v="145", "Not A(Brand";v="24"`},
		{"sec-ch-ua-mobile", "Sec-Ch-Ua-Mobile", "?0"},
		{"sec-ch-ua-platform", "Sec-Ch-Ua-Platform", `"macOS"`},
		{"sec-fetch-dest", "Sec-Fetch-Dest", "video"},
		{"sec-fetch-mode", "Sec-Fetch-Mode", "no-cors"},
		{"sec-fetch-site", "Sec-Fetch-Site", "cross-site"},
		{"cache-control", "Cache-Control", "no-cache"},
		{"pragma", "Pragma", "no-cache"},
		{"priority", "Priority", "i"},
	}

	for _, c := range checks {
		got := strings.Join(capturedHeaders.Values(c.key), ", ")
		if got == "" {
			t.Errorf("missing header %q (key=%s)", c.name, c.key)
		} else if got != c.expect {
			t.Errorf("header %q: got %q, want %q", c.name, got, c.expect)
		}
	}
}

// TestHTTPExtractorBrowserHeadersDisabled verifies browser headers are not injected when disabled.
func TestHTTPExtractorBrowserHeadersDisabled(t *testing.T) {
	var capturedHeaders http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")

	ext := newHTTPExtractor(t)
	ext.SetBrowserHeaders(false) // default should be false
	req := &download.Request{
		URL:      ts.URL,
		SavePath: dest,
	}
	err := ext.Extract(t.Context(), req)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	if v, ok := capturedHeaders["Sec-Ch-Ua"]; ok {
		t.Errorf("expected no Sec-Ch-Ua header when browser headers disabled, got: %v", v)
	}
	if v, ok := capturedHeaders["Sec-Fetch-Dest"]; ok {
		t.Errorf("expected no Sec-Fetch-Dest header when browser headers disabled, got: %v", v)
	}
}

// TestHTTPExtractorCustomHeaderOverridesBrowser verifies user-provided headers
// take precedence over injected browser headers.
func TestHTTPExtractorCustomHeaderOverridesBrowser(t *testing.T) {
	var capturedUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.UserAgent()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")

	ext := newHTTPExtractor(t)
	ext.SetBrowserHeaders(true)
	req := &download.Request{
		URL:      ts.URL,
		SavePath: dest,
		Headers: map[string]string{
			"User-Agent": "CustomAgent/2.0",
		},
	}
	err := ext.Extract(t.Context(), req)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if capturedUA != "CustomAgent/2.0" {
		t.Errorf("User-Agent: got %q, want CustomAgent/2.0", capturedUA)
	}
}
