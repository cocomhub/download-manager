// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/download/extractor"
)

func TestHLSExtractorName(t *testing.T) {
	ex := extractor.NewHLSExtractor()
	if ex.Name() != "hls" {
		t.Errorf("expected 'hls', got %s", ex.Name())
	}
}

func TestHLSExtractorMatchM3U8(t *testing.T) {
	ex := extractor.NewHLSExtractor()
	if !ex.Match(t.Context(), "http://example.com/stream.m3u8") {
		t.Error("HLSExtractor should match .m3u8 URLs")
	}
	if !ex.Match(t.Context(), "http://example.com/playlist.M3U8") {
		t.Error("HLSExtractor should match .M3U8 URLs (case-insensitive)")
	}
	if ex.Match(t.Context(), "http://example.com/file.mp4") {
		t.Error("HLSExtractor should NOT match non-m3u8 URLs")
	}
}

func TestHLSExtractorNoFFmpeg(t *testing.T) {
	ex := extractor.NewHLSExtractor(extractor.WithHLSMode("ffmpeg"), extractor.WithFFmpegPath("/nonexistent/ffmpeg"))
	err := ex.Extract(t.Context(), &download.Request{
		URL:      "http://example.com/stream.m3u8",
		SavePath: "/tmp/output.mp4",
	})
	if err == nil {
		t.Fatal("expected error when ffmpeg is not available")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestConcatM3U8Segments(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "parts")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 本地 m3u8（M3U8DEngine 改写后：分片行 = workdir 下 hash.ts）
	m3u8 := `#EXTM3U
#EXTINF:4.0,
seg_aaa.ts
#EXTINF:4.0,
seg_bbb.ts
#EXT-X-ENDLIST
`
	// 模拟分片文件
	for name, content := range map[string]string{
		"seg_aaa.ts": "AAA",
		"seg_bbb.ts": "BBB",
	} {
		if err := os.WriteFile(filepath.Join(workDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m3u8Path := filepath.Join(dir, "master.m3u8")
	if err := os.WriteFile(m3u8Path, []byte(m3u8), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "output.ts")
	if err := extractor.ConcatM3U8Segments(m3u8Path, workDir, out); err != nil {
		t.Fatalf("concat: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "AAABBB" {
		t.Fatalf("concat result = %q, want %q", string(got), "AAABBB")
	}
}

func TestConcatM3U8Segments_NoSegments(t *testing.T) {
	dir := t.TempDir()
	m3u8Path := filepath.Join(dir, "empty.m3u8")
	if err := os.WriteFile(m3u8Path, []byte("#EXTM3U\n#EXT-X-ENDLIST\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := extractor.ConcatM3U8Segments(m3u8Path, dir, filepath.Join(dir, "out.ts"))
	if err == nil {
		t.Fatal("expected error for empty m3u8")
	}
}

func TestConcatM3U8Segments_MasterListRecursive(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "parts")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 主列表：3 档位 → 递归选最后（最高清）
	master := `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360
sub_640.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1400000,RESOLUTION=842x480
sub_842.m3u8
`
	// 子列表（842）：分片引用
	sub := `#EXTM3U
#EXTINF:4.0,
seg_aaa.ts
#EXTINF:4.0,
seg_bbb.ts
`
	// 模拟分片
	for name, content := range map[string]string{
		"seg_aaa.ts": "AAA",
		"seg_bbb.ts": "BBB",
		"sub_640.m3u8": "#EXTM3U\n#EXTINF:4.0,\nlow.ts\n",
		"sub_842.m3u8": sub,
	} {
		if err := os.WriteFile(filepath.Join(workDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// 主列表文件
	masterPath := filepath.Join(dir, "master.m3u8")
	if err := os.WriteFile(masterPath, []byte(master), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.ts")
	if err := extractor.ConcatM3U8Segments(masterPath, workDir, out); err != nil {
		t.Fatalf("concat: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "AAABBB" {
		t.Fatalf("concat = %q, want AAABBB (highest quality sublist)", string(got))
	}
}
