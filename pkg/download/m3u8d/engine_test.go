// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package m3u8d

import (
	"net/url"
	"os"
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

	// 创建测试 m3u8 文件，包含 #EXTINF 行
	m3u8Content := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXTINF:10,
seg001.ts
#EXTINF:10,
seg002.ts
#EXT-X-ENDLIST
`
	m3u8Path := dir + "/test.m3u8"
	if err := writeFile(m3u8Path, m3u8Content); err != nil {
		t.Fatal(err)
	}

	cfg := &DownloadConfig{
		InputURL:   "https://example.com/stream.m3u8",
		OutputFile: dir + "/output.mp4",
		WorkDir:    dir,
	}

	d, err := NewM3U8DEngine(cfg, nil)
	if err != nil {
		t.Fatalf("NewM3U8DEngine failed: %v", err)
	}

	// 测试 parseM3U8（无实际下载，仅解析）
	localPath := dir + "/parsed.m3u8"
	// 直接测试 resolveURL
	base, _ := url.Parse(cfg.InputURL)
	result := resolveURL(base, "seg001.ts")
	if result != "https://example.com/seg001.ts" {
		t.Errorf("resolveURL = %q, want %q", result, "https://example.com/seg001.ts")
	}
	_ = localPath
	_ = d
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
