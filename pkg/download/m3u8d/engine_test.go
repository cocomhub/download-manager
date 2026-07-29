// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package m3u8d

import (
	"net/url"
	"testing"
)

func TestResolveURL(t *testing.T) {
	base, _ := url.Parse("https://example.com/path/stream.m3u8")

	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"absolute URL", "https://cdn.example.com/seg001.ts", "https://cdn.example.com/seg001.ts"},
		{"relative segment", "seg001.ts", "https://example.com/path/seg001.ts"},
		{"relative subdir", "../seg001.ts", "https://example.com/seg001.ts"},
		{"empty ref", "", ""},
		{"query-based", "seg001.ts?token=abc", "https://example.com/path/seg001.ts?token=abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveURL(base, tt.ref)
			if got != tt.want {
				t.Errorf("resolveURL(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestExtractKeyURL(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   string
		wantOK bool
	}{
		{"standard KEY", `#EXT-X-KEY:METHOD=AES-128,URI="https://keys.example.com/key.bin"`, "https://keys.example.com/key.bin", true},
		{"relative key URI", `#EXT-X-KEY:METHOD=AES-128,URI="key.bin"`, "key.bin", true},
		{"no KEY line", "#EXTINF:10,", "", false},
		{"KEY without URI", "#EXT-X-KEY:METHOD=NONE", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractKeyURL(tt.line)
			if ok != tt.wantOK {
				t.Errorf("extractKeyURL() ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("extractKeyURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMarkAndIsDownloaded(t *testing.T) {
	d := &M3U8DEngine{
		downloaded: make(map[string]bool),
	}

	d.markAsDownloaded("https://example.com/seg001.ts")
	d.markAsDownloaded("https://example.com/seg002.ts")

	if !d.isAlreadyDownloaded("https://example.com/seg001.ts") {
		t.Error("expected seg001.ts to be marked as downloaded")
	}
	if !d.isAlreadyDownloaded("https://example.com/seg002.ts") {
		t.Error("expected seg002.ts to be marked as downloaded")
	}
	if d.isAlreadyDownloaded("https://example.com/seg003.ts") {
		t.Error("expected seg003.ts to NOT be marked as downloaded")
	}
}

func TestParseM3U8SingleLevel(t *testing.T) {
	dir := t.TempDir()

	cfg := &DownloadConfig{
		InputURL:   "https://example.com/stream.m3u8",
		OutputFile: dir + "/output.mp4",
		WorkDir:    dir,
	}

	d, err := NewM3U8DEngine(cfg, nil)
	if err != nil {
		t.Fatalf("NewM3U8DEngine failed: %v", err)
	}

	// Test processM3U8Line directly to verify TS segment extraction
	base, _ := url.Parse(cfg.InputURL)

	tests := []struct {
		name     string
		line     string
		wantTask int
		wantType string
	}{
		{"directive line", "#EXTINF:10,", 0, ""},
		{"ts segment", "seg001.ts", 1, "ts"},
		{"ts segment with path", "sub/seg002.ts", 1, "ts"},
		{"key file", "key.bin", 1, "key"},
		{"empty line", "", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, tasks, err := d.processM3U8Line(t.Context(), base, tt.line, 0)
			if err != nil {
				t.Fatalf("processM3U8Line failed: %v", err)
			}
			if len(tasks) != tt.wantTask {
				t.Errorf("expected %d tasks, got %d", tt.wantTask, len(tasks))
			}
			if tt.wantType != "" && len(tasks) > 0 && tasks[0].Type != tt.wantType {
				t.Errorf("expected type %q, got %q", tt.wantType, tasks[0].Type)
			}
		})
	}

	// Test path traversal protection
	_, _, err = d.processM3U8Line(t.Context(), base, "../escape.ts", 0)
	if err == nil {
		t.Error("expected error for path traversal, got nil")
	}
}
