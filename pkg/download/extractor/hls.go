// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cocomhub/download-manager/pkg/download"
)

// HLSMode 表示 HLS 下载模式。
type HLSMode string

const (
	HLSModeFFmpeg HLSMode = "ffmpeg"
	HLSModeM3U8D  HLSMode = "m3u8d"
)

// compile-time interface check
var _ download.Extractor = (*HLSExtractor)(nil)

// HLSExtractor 处理 HLS (m3u8) 流媒体下载。
type HLSExtractor struct {
	mode       HLSMode
	ffmpegPath string
	ffmpegArgs []string
	userAgent  string
}

// NewHLSExtractor 创建 HLSExtractor。
func NewHLSExtractor(opts ...HLSOption) *HLSExtractor {
	e := &HLSExtractor{
		mode:       HLSModeFFmpeg,
		ffmpegPath: "ffmpeg",
		ffmpegArgs: []string{"-c", "copy", "-bsf:a", "aac_adtstoasc", "-movflags", "+faststart", "-f", "mp4"},
		userAgent:  DefaultWgetUserAgent,
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// HLSOption 是 HLSExtractor 的配置函数。
type HLSOption func(*HLSExtractor)

// WithHLSMode 设置 HLS 下载模式（ffmpeg / m3u8d）。
func WithHLSMode(mode string) HLSOption {
	return func(e *HLSExtractor) { e.mode = HLSMode(mode) }
}

// WithFFmpegPath 设置 ffmpeg 可执行文件路径。
func WithFFmpegPath(path string) HLSOption { return func(e *HLSExtractor) { e.ffmpegPath = path } }

// WithFFmpegArgs 设置 ffmpeg 额外参数。
func WithFFmpegArgs(args []string) HLSOption { return func(e *HLSExtractor) { e.ffmpegArgs = args } }

// WithHLSUserAgent 设置自定义 User-Agent。
func WithHLSUserAgent(ua string) HLSOption { return func(e *HLSExtractor) { e.userAgent = ua } }

// SetTransport 是空操作，HLSExtractor 通过 ffmpeg exec 或 m3u8d 下载，不依赖 Transport。
func (e *HLSExtractor) SetTransport(_ download.Transport) {}

func (e *HLSExtractor) Name() string { return "hls" }

// Match 判断 URL 是否为 .m3u8 后缀（不区分大小写）。
func (e *HLSExtractor) Match(_ context.Context, url string) bool {
	return strings.Contains(strings.ToLower(url), ".m3u8")
}

// Extract 根据模式选择 HLS 下载方式。
func (e *HLSExtractor) Extract(ctx context.Context, req *download.Request) error {
	switch e.mode {
	case HLSModeFFmpeg:
		return e.downloadWithFFmpeg(ctx, req)
	case HLSModeM3U8D:
		return e.downloadWithM3U8D(ctx, req)
	default:
		return e.downloadWithFFmpeg(ctx, req)
	}
}

func (e *HLSExtractor) downloadWithFFmpeg(ctx context.Context, req *download.Request) error {
	rPath := req.SavePath
	dir := filepath.Dir(rPath)

	// Validate args to prevent argv injection
	if strings.HasPrefix(rPath, "-") {
		return fmt.Errorf("hls: invalid save path (starts with '-')")
	}
	if strings.HasPrefix(req.URL, "-") {
		return fmt.Errorf("hls: invalid URL (starts with '-')")
	}
	if !strings.HasPrefix(strings.ToLower(req.URL), "http://") && !strings.HasPrefix(strings.ToLower(req.URL), "https://") {
		return fmt.Errorf("hls: invalid URL scheme")
	}
	for _, v := range []string{req.Headers["Referer"], req.Headers["Cookie"]} {
		if strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("hls: invalid header value contains CR/LF")
		}
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("hls: failed to create directory: %w", err)
	}

	ffmpeg := e.ffmpegPath
	if path, err := exec.LookPath(ffmpeg); err == nil {
		ffmpeg = path
	} else {
		return fmt.Errorf("hls: ffmpeg not found: %w", err)
	}

	args := []string{"-y"}
	if e.userAgent != "" {
		args = append(args, "-user_agent", e.userAgent)
	}

	var headerLines []string
	if v := req.Headers["Referer"]; v != "" {
		headerLines = append(headerLines, fmt.Sprintf("Referer: %s", v))
	}
	if v := req.Headers["Cookie"]; v != "" {
		headerLines = append(headerLines, fmt.Sprintf("Cookie: %s", v))
	}
	if len(headerLines) > 0 {
		args = append(args, "-headers", strings.Join(headerLines, "\r\n"))
	}

	args = append(args, "-i", req.URL)
	args = append(args, e.ffmpegArgs...)
	args = append(args, rPath)

	slog.Info("Starting HLS download", "downloader", "ffmpeg", "url", req.URL)

	cmd := exec.CommandContext(ctx, ffmpeg, args...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("hls: failed to attach stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("hls: ffmpeg start failed: %w", err)
	}

	// drain stderr to avoid blocking
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := stderr.Read(buf)
			if err != nil {
				break
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("hls: ffmpeg execution failed: %w", err)
	}

	if req.OnProgress != nil {
		// 用实际文件大小填充 downloaded 与 total，避免传零值。
		var size int64
		if info, err := os.Stat(rPath); err == nil {
			size = info.Size()
		}
		req.OnProgress(100, size, size)
	}
	if info, err := os.Stat(rPath); err == nil {
		if req.Result == nil {
			req.Result = &download.DownloadResult{}
		}
		req.Result.TotalSize = info.Size()
	}
	return nil
}

func (e *HLSExtractor) downloadWithM3U8D(_ context.Context, _ *download.Request) error {
	return fmt.Errorf("hls: m3u8d mode not yet implemented in HLSExtractor")
}
