// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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

// TestDownloadWithM3U8D_NoFFmpegFallbackConcat 验证：m3u8d 模式无 ffmpeg 时
// 回退纯 TS 拼接（输出内容 = 分片顺序拼接）。
func TestDownloadWithM3U8D_NoFFmpegFallbackConcat(t *testing.T) {
	// httptest 提供 m3u8 + 分片
	mux := http.NewServeMux()
	mux.HandleFunc("/stream.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:4.0,\nseg0.ts\n#EXTINF:4.0,\nseg1.ts\n#EXT-X-ENDLIST\n"))
	})
	mux.HandleFunc("/seg0.ts", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("SEG0-DATA"))
	})
	mux.HandleFunc("/seg1.ts", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("SEG1-DATA"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "out.mp4")
	// 注入不存在的 ffmpeg 路径 → 触发回退拼接
	ex := extractor.NewHLSExtractor(extractor.WithHLSMode("m3u8d"), extractor.WithFFmpegPath(filepath.Join(dir, "no-ffmpeg")))
	err := ex.Extract(t.Context(), &download.Request{
		URL:      srv.URL + "/stream.m3u8",
		SavePath: out,
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "SEG0-DATASEG1-DATA" {
		t.Fatalf("concat output = %q, want SEG0-DATASEG1-DATA", string(got))
	}
}

// TestDownloadWithM3U8D_FFmpegConvert 验证：有 ffmpeg 时走转封装（mock ffmpeg 被调用）。
// mock ffmpeg 直接把输入 m3u8 复制为输出（模拟转封装成功）。
func TestDownloadWithM3U8D_FFmpegConvert(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mock ffmpeg 脚本依赖 sh（linux/mac）")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/stream.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:4.0,\nseg0.ts\n#EXT-X-ENDLIST\n"))
	})
	mux.HandleFunc("/seg0.ts", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("SEG0-DATA"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "out.mp4")
	marker := filepath.Join(dir, "ffmpeg-called.marker")
	// mock ffmpeg：验证被调用（写 marker）+ 产出输出文件（模拟 ConvertToMP4 成功）
	mockFF := filepath.Join(dir, "ffmpeg")
	// mock ffmpeg：验证被调用（touch marker）+ 产出输出文件（模拟 ConvertToMP4 成功）
	// marker 路径硬编码（不依赖环境变量）；输出解析 -- 后参数
	script := "#!/bin/sh\n" +
		"touch '" + marker + "'\n" +
		"out=\"\"\n" +
		"skip=0\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$skip\" = \"1\" ]; then out=\"$a\"; break; fi\n" +
		"  [ \"$a\" = \"--\" ] && skip=1\n" +
		"done\n" +
		"echo mock > \"$out\"\n"
	if err := os.WriteFile(mockFF, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	ex := extractor.NewHLSExtractor(extractor.WithHLSMode("m3u8d"), extractor.WithFFmpegPath(mockFF))
	err := ex.Extract(t.Context(), &download.Request{
		URL:      srv.URL + "/stream.m3u8",
		SavePath: out,
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("ffmpeg was NOT called (marker missing): %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "mock\n" {
		t.Fatalf("output = %q, want mock ffmpeg output", string(got))
	}
}
