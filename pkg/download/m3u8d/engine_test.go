// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package m3u8d

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
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
			got, err := resolveURL(base, tt.ref)
			if err != nil {
				t.Fatalf("resolveURL(%q) returned error: %v", tt.ref, err)
			}
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
		OutputFile: filepath.Join(dir, "output.mp4"),
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
		{"jpeg disguised segment", "video0.jpeg", 1, "ts"},
		{"jpg disguised segment", "video1.jpg", 1, "ts"},
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

func TestParseM3U8_MasterOnlyHighestQuality(t *testing.T) {
	dir := t.TempDir()
	// mock m3u8 下载（httptest server 返回主列表 + 子列表）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "master.m3u8"):
			w.Write([]byte(`#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360
sub640.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1400000,RESOLUTION=842x480
sub842.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2800000,RESOLUTION=1280x720
sub1280.m3u8
`))
		case strings.HasSuffix(r.URL.Path, "sub640.m3u8"):
			w.Write([]byte("#EXTM3U\n#EXTINF:4.0,\nlow0.ts\n#EXTINF:4.0,\nlow1.ts\n"))
		case strings.HasSuffix(r.URL.Path, "sub842.m3u8"):
			w.Write([]byte("#EXTM3U\n#EXTINF:4.0,\nmid0.ts\n"))
		case strings.HasSuffix(r.URL.Path, "sub1280.m3u8"):
			w.Write([]byte("#EXTM3U\n#EXTINF:4.0,\nhigh0.ts\n#EXTINF:4.0,\nhigh1.ts\n#EXTINF:4.0,\nhigh2.ts\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := &DownloadConfig{
		InputURL:   srv.URL + "/master.m3u8",
		OutputFile: filepath.Join(dir, "out.mp4"),
		WorkDir:    dir,
		MinFiles:   1,
	}
	d, err := NewM3U8DEngine(cfg, nil)
	if err != nil {
		t.Fatalf("NewM3U8DEngine: %v", err)
	}
	tasks, err := d.parseM3U8(t.Context(), cfg.InputURL, filepath.Join(dir, "master.m3u8"), 0)
	if err != nil {
		t.Fatalf("parseM3U8: %v", err)
	}
	// 只应下载最高档（1280x720 = 3 分片）
	if len(tasks) != 3 {
		t.Fatalf("tasks = %d, want 3 (highest quality only, no 640/842)", len(tasks))
	}
	// 任务 URL 应都对应最高档分片（high 名，来自 sub1280 子列表）
	for _, tk := range tasks {
		if !strings.Contains(tk.URL, "high") {
			t.Fatalf("task URL %s not from highest quality sublist", tk.URL)
		}
	}
}
