// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package m3u8d

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// masterPlaylist 构造多档位主列表（档位按码率/分辨率升序）。
func masterPlaylist() string {
	return `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360
sub640.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1400000,RESOLUTION=842x480
sub842.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2800000,RESOLUTION=1280x720
sub1280.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080
sub1080.m3u8
`
}

// masterTestServer 返回多档位主列表 + 最高档(1080p)子列表的测试服务器。
func masterTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "master.m3u8"):
			w.Write([]byte(masterPlaylist()))
		case strings.HasSuffix(r.URL.Path, "sub1080.m3u8"):
			w.Write([]byte("#EXTM3U\n#EXTINF:4.0,\nh0.ts\n#EXTINF:4.0,\nh1.ts\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	return srv
}

// TestParseM3U8_RecordsHighestResolution 验证主列表解析记录最高档位分辨率（1080）。
func TestParseM3U8_RecordsHighestResolution(t *testing.T) {
	srv := masterTestServer(t)
	defer srv.Close()
	dir := t.TempDir()

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
	if _, err := d.parseM3U8(t.Context(), cfg.InputURL, filepath.Join(dir, "master.m3u8"), 0); err != nil {
		t.Fatalf("parseM3U8: %v", err)
	}
	if d.resolutionHeight != 1080 {
		t.Fatalf("resolutionHeight = %d, want 1080 (highest quality)", d.resolutionHeight)
	}
}

// TestResolveCRF_ByResolution 验证按分辨率选 CRF：720→40、1080→34、超高清→更低 CRF。
func TestResolveCRF_ByResolution(t *testing.T) {
	srv := masterTestServer(t)
	defer srv.Close()
	dir := t.TempDir()

	cfg := &DownloadConfig{
		InputURL:   srv.URL + "/master.m3u8",
		OutputFile: filepath.Join(dir, "out.mp4"),
		WorkDir:    dir,
		MinFiles:   1,
		ResolutionCRF: map[int]int{
			720:  40,
			1080: 34,
			2160: 28,
		},
		DefaultCRF: 40,
	}
	d, err := NewM3U8DEngine(cfg, nil)
	if err != nil {
		t.Fatalf("NewM3U8DEngine: %v", err)
	}
	if _, err := d.parseM3U8(t.Context(), cfg.InputURL, filepath.Join(dir, "master.m3u8"), 0); err != nil {
		t.Fatalf("parseM3U8: %v", err)
	}
	// 主列表最高 1080 → 应选 34
	if got := d.resolveCRF(); got != 34 {
		t.Fatalf("resolveCRF = %d, want 34 (1080p)", got)
	}
}

// TestResolveCRF_FallbackDefault 验证无档位（单列表）时回退 DefaultCRF。
func TestResolveCRF_FallbackDefault(t *testing.T) {
	dir := t.TempDir()
	cfg := &DownloadConfig{
		InputURL:   "https://example.com/stream.m3u8",
		OutputFile: filepath.Join(dir, "out.mp4"),
		WorkDir:    dir,
		MinFiles:   1,
		ResolutionCRF: map[int]int{
			720: 40,
		},
		DefaultCRF: 36,
	}
	d, err := NewM3U8DEngine(cfg, nil)
	if err != nil {
		t.Fatalf("NewM3U8DEngine: %v", err)
	}
	// 未解析 → resolutionHeight=0 → 无匹配 key → DefaultCRF
	if got := d.resolveCRF(); got != 36 {
		t.Fatalf("resolveCRF = %d, want 36 (default)", got)
	}
}

// TestResolveCRF_FallbackZeroDefault 验证 DefaultCRF 未显式设置（0）时
// 回退到包级默认 defaultCRF，避免生成 "-crf 0" 导致码率爆炸。
func TestResolveCRF_FallbackZeroDefault(t *testing.T) {
	dir := t.TempDir()
	cfg := &DownloadConfig{
		InputURL:      "https://example.com/stream.m3u8",
		OutputFile:    filepath.Join(dir, "out.mp4"),
		WorkDir:       dir,
		FFmpegArgs:    []string{"-c:v", "libsvtav1", "-crf", "{crf}"},
		ResolutionCRF: nil,
		// DefaultCRF 故意不设（0），验证兜底
	}
	d, err := NewM3U8DEngine(cfg, nil)
	if err != nil {
		t.Fatalf("NewM3U8DEngine: %v", err)
	}
	if got := d.resolveCRF(); got != defaultCRF {
		t.Fatalf("resolveCRF = %d, want defaultCRF %d when DefaultCRF unset", got, defaultCRF)
	}
}

// TestResolveFFmpegArgs_ReplaceCRFPlaceholder 验证 {crf} 占位符按分辨率替换。
func TestResolveFFmpegArgs_ReplaceCRFPlaceholder(t *testing.T) {
	srv := masterTestServer(t)
	defer srv.Close()
	dir := t.TempDir()

	cfg := &DownloadConfig{
		InputURL:   srv.URL + "/master.m3u8",
		OutputFile: filepath.Join(dir, "out.mp4"),
		WorkDir:    dir,
		MinFiles:   1,
		FFmpegArgs: []string{"-c:v", "libsvtav1", "-crf", "{crf}", "-b:v", "0", "-preset", "8"},
		ResolutionCRF: map[int]int{
			720:  40,
			1080: 34,
		},
		DefaultCRF: 40,
	}
	d, err := NewM3U8DEngine(cfg, nil)
	if err != nil {
		t.Fatalf("NewM3U8DEngine: %v", err)
	}
	if _, err := d.parseM3U8(t.Context(), cfg.InputURL, filepath.Join(dir, "master.m3u8"), 0); err != nil {
		t.Fatalf("parseM3U8: %v", err)
	}
	got := d.resolveFFmpegArgs()
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "-crf 34") {
		t.Fatalf("resolveFFmpegArgs = %v, want -crf 34 (1080p), got %q", got, joined)
	}
	if strings.Contains(joined, "{crf}") {
		t.Fatalf("resolveFFmpegArgs left placeholder {crf}: %v", got)
	}
}

// TestResolveFFmpegArgs_NoPlaceholderPassthrough 验证无 {crf} 占位符时原样返回。
func TestResolveFFmpegArgs_NoPlaceholderPassthrough(t *testing.T) {
	dir := t.TempDir()
	cfg := &DownloadConfig{
		InputURL:   "https://example.com/stream.m3u8",
		OutputFile: filepath.Join(dir, "out.mp4"),
		WorkDir:    dir,
		FFmpegArgs: []string{"-c", "copy"},
	}
	d, err := NewM3U8DEngine(cfg, nil)
	if err != nil {
		t.Fatalf("NewM3U8DEngine: %v", err)
	}
	got := d.resolveFFmpegArgs()
	if len(got) != 2 || got[0] != "-c" || got[1] != "copy" {
		t.Fatalf("resolveFFmpegArgs = %v, want passthrough [-c copy]", got)
	}
}
