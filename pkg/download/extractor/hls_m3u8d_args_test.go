// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/download/extractor"
)

// captureFFmpeg 记录一次 ffmpeg 调用的完整参数到 argsFile，并产出输出文件。
// mock 脚本：把 "$@" 逐行写入 argsFile，模拟产物写入 -- 后的输出路径。
func captureFFmpeg(t *testing.T, argsFile, outFile string) string {
	t.Helper()
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do printf '%s\\n' \"$a\" >> '" + argsFile + "'; done\n" +
		"out=\"\"\n" +
		"skip=0\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$skip\" = \"1\" ]; then out=\"$a\"; break; fi\n" +
		"  [ \"$a\" = \"--\" ] && skip=1\n" +
		"done\n" +
		"echo mock > \"$out\"\n"
	mock := filepath.Join(filepath.Dir(outFile), "ffmpeg")
	if err := os.WriteFile(mock, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return mock
}

func m3u8dTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/stream.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:4.0,\nseg0.ts\n#EXT-X-ENDLIST\n"))
	})
	mux.HandleFunc("/seg0.ts", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("SEG0-DATA"))
	})
	srv := httptest.NewServer(mux)
	return srv
}

// runM3U8DExtract 执行一次 m3u8d Extract，读取 mock ffmpeg 记录到的参数。
func runM3U8DExtract(t *testing.T, opts ...extractor.HLSOption) []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("mock ffmpeg 脚本依赖 sh（linux/mac）")
	}
	srv := m3u8dTestServer(t)
	defer srv.Close()
	dir := t.TempDir()
	out := filepath.Join(dir, "out.mp4")
	argsFile := filepath.Join(dir, "ffmpeg-args.txt")
	mock := captureFFmpeg(t, argsFile, out)
	opts = append([]extractor.HLSOption{
		extractor.WithHLSMode("m3u8d"),
		extractor.WithFFmpegPath(mock),
	}, opts...)
	ex := extractor.NewHLSExtractor(opts...)
	if err := ex.Extract(t.Context(), &download.Request{
		URL:      srv.URL + "/stream.m3u8",
		SavePath: out,
	}); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	b, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read ffmpeg args file: %v", err)
	}
	var args []string
	for l := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
		if l != "" {
			args = append(args, l)
		}
	}
	if len(args) == 0 {
		t.Fatalf("mock ffmpeg not invoked (no args recorded)")
	}
	return args
}

// TestM3U8D_DefaultPassesStreamCopy 验证 m3u8d 模式转封装默认保留 -c copy（流复制，不重编码）。
// 回归：downloadWithM3U8D 曾未透传 FFmpegArgs → ffmpeg 收到 nil 参数 → 默认重编码成 h264
// （体积膨胀 + CPU 满载）。默认必须包含 -c copy。
func TestM3U8D_DefaultPassesStreamCopy(t *testing.T) {
	args := runM3U8DExtract(t)
	for _, want := range []string{"-c", "copy"} {
		if !contains(args, want) {
			t.Fatalf("m3u8d ffmpeg args missing %q, got: %v", want, args)
		}
	}
	// 转封装阶段也应保留 fMP4 流式标记（低内存）
	joined := strings.Join(args, " ")
	for _, flag := range []string{"frag_keyframe+empty_moov+default_base_moof", "-max_muxing_queue_size", "-f", "mp4"} {
		if !strings.Contains(joined, flag) {
			t.Errorf("m3u8d ffmpeg args missing %q: %v", flag, args)
		}
	}
}

// TestM3U8D_CustomFFmpegArgsReplaceDefault 验证 WithFFmpegArgs 提供的参数
// 完全替换默认 -c copy（供转码/重编码使用），且不被默认参数并集污染。
func TestM3U8D_CustomFFmpegArgsReplaceDefault(t *testing.T) {
	custom := []string{"-c:v", "libx265", "-crf", "28"}
	args := runM3U8DExtract(t, extractor.WithFFmpegArgs(custom))
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "libx265") || !strings.Contains(joined, "-crf") {
		t.Fatalf("m3u8d ffmpeg args missing custom transcode flags: %v", args)
	}
	// 默认 stream copy 必须被替换（不能同时存在 -c copy）
	if contains(args, "copy") {
		t.Errorf("custom args should REPLACE default -c copy, but still present: %v", args)
	}
}

func contains(ss []string, want string) bool {
	return slices.Contains(ss, want)
}
