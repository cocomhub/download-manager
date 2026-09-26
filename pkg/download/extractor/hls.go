// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"bufio"
	"context"
	"fmt"
	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/download/m3u8d"
	"github.com/cocomhub/download-manager/pkg/logutil"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var reFFmpegTime = regexp.MustCompile(`time=(\d+):(\d+):(\d+)\.(\d+)`)

// HLSMode 表示 HLS 下载模式。
type HLSMode string

const (
	HLSModeFFmpeg HLSMode = "ffmpeg"
	// TODO: 实现 m3u8d 模式。当前仅 ffmpeg 模式可用。
	HLSModeM3U8D HLSMode = "m3u8d"
)

// compile-time interface check
var _ download.Extractor = (*HLSExtractor)(nil)
var _ download.Canceller = (*HLSExtractor)(nil)
var _ download.TransportSetter = (*HLSExtractor)(nil)
var _ download.SelectorSetter = (*HLSExtractor)(nil)

// HLSExtractor 处理 HLS (m3u8) 流媒体下载。
type HLSExtractor struct {
	mode          HLSMode
	ffmpegPath    string
	ffmpegArgs    []string
	ffmpegTimeout time.Duration
	userAgent     string
	active        sync.Map // map[string]context.CancelFunc
}

// NewHLSExtractor 创建 HLSExtractor。
func NewHLSExtractor(opts ...HLSOption) *HLSExtractor {
	e := &HLSExtractor{
		mode:          HLSModeFFmpeg,
		ffmpegPath:    "ffmpeg",
		ffmpegArgs:    []string{"-c", "copy", "-bsf:a", "aac_adtstoasc", "-movflags", "+faststart", "-f", "mp4"},
		ffmpegTimeout: 5 * time.Minute,
		userAgent:     DefaultWgetUserAgent,
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

// WithFFmpegTimeout 设置 ffmpeg 执行超时时间。
func WithFFmpegTimeout(d time.Duration) HLSOption {
	return func(e *HLSExtractor) { e.ffmpegTimeout = d }
}

// WithHLSUserAgent 设置自定义 User-Agent。
func WithHLSUserAgent(ua string) HLSOption { return func(e *HLSExtractor) { e.userAgent = ua } }

// SetTransport is a no-op: HLSExtractor downloads via ffmpeg exec or m3u8d,
// not through a Go Transport. Implemented for download.TransportSetter interface.
func (e *HLSExtractor) SetTransport(_ download.Transport) {}

// SetSelector 注入 Selector 实例（当前为 no-op，HLS 通过 ffmpeg 执行）。
func (e *HLSExtractor) SetSelector(_ download.Selector) {}

// Cancel 取消正在进行的 HLS 下载。
func (e *HLSExtractor) Cancel(url string) error {
	if v, ok := e.active.Load(url); ok {
		if cancel, ok := v.(context.CancelFunc); ok {
			cancel()
		}
		e.active.Delete(url)
	}
	return nil
}

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
	if err := validateHLSParams(req); err != nil {
		return err
	}
	rPath := req.SavePath
	if err := os.MkdirAll(filepath.Dir(rPath), 0755); err != nil {
		return fmt.Errorf("hls: failed to create directory: %w", err)
	}
	ffmpeg := e.ffmpegPath
	if path, err := exec.LookPath(ffmpeg); err != nil {
		return fmt.Errorf("hls: ffmpeg not found: %w", err)
	} else {
		ffmpeg = path
	}
	return e.executeFFmpeg(ctx, ffmpeg, e.buildFFmpegArgs(req), rPath, req)
}

func validateHLSParams(req *download.Request) error {
	if strings.HasPrefix(req.SavePath, "-") {
		return fmt.Errorf("hls: invalid save path (starts with '-')")
	}
	if strings.ContainsAny(req.SavePath, "\r\n") {
		return fmt.Errorf("hls: invalid save path contains CR/LF")
	}
	if strings.HasPrefix(req.URL, "-") {
		return fmt.Errorf("hls: invalid URL (starts with '-')")
	}
	lowerURL := strings.ToLower(req.URL)
	if !strings.HasPrefix(lowerURL, "http://") && !strings.HasPrefix(lowerURL, "https://") {
		return fmt.Errorf("hls: invalid URL scheme")
	}
	// 验证所有 header 的 CR/LF 和 - 前缀（防止通过 -headers 注入）
	for k, v := range req.Headers {
		if strings.ContainsAny(k, "\r\n") || strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("hls: invalid header contains CR/LF: %q", k)
		}
		if strings.HasPrefix(k, "-") || strings.HasPrefix(v, "-") {
			return fmt.Errorf("hls: header key/value starts with '-'")
		}
	}
	// 验证 URL 不包含 CR/LF
	if strings.ContainsAny(req.URL, "\r\n") {
		return fmt.Errorf("hls: invalid URL contains CR/LF")
	}
	return nil
}

func (e *HLSExtractor) buildFFmpegArgs(req *download.Request) []string {
	args := []string{"-y"}
	if e.userAgent != "" {
		args = append(args, "-user_agent", e.userAgent)
	}
	var headerLines []string
	for k, v := range req.Headers {
		if k == "User-Agent" {
			continue // 已通过 -user_agent 传递
		}
		headerLines = append(headerLines, fmt.Sprintf("%s: %s", k, v))
	}
	if len(headerLines) > 0 {
		args = append(args, "-headers", strings.Join(headerLines, "\r\n"))
	}
	args = append(args, "-i", req.URL)
	args = append(args, e.ffmpegArgs...)
	args = append(args, "--", req.SavePath)
	return args
}

func (e *HLSExtractor) executeFFmpeg(ctx context.Context, ffmpeg string, args []string, rPath string, req *download.Request) error {
	slog.Info("Starting HLS download", "downloader", "ffmpeg", logutil.LogKeyURL, req.URL)

	dlCtx, dlCancel := context.WithCancel(ctx)
	defer e.active.Delete(req.URL)
	defer dlCancel()
	e.active.Store(req.URL, dlCancel)

	cmd := exec.CommandContext(dlCtx, ffmpeg, args...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("hls: failed to attach stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("hls: ffmpeg start failed: %w", err)
	}

	// drain stderr in background goroutine, respond to context cancellation
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			slog.Debug("ffmpeg stderr", "line", line)

			if req.OnProgress != nil && req.TrackProgress {
				if matches := reFFmpegTime.FindStringSubmatch(line); matches != nil {
					h, _ := strconv.Atoi(matches[1])
					m, _ := strconv.Atoi(matches[2])
					s, _ := strconv.Atoi(matches[3])
					totalSecs := float64(h*3600 + m*60 + s)
					req.OnProgress(totalSecs, 0, 0)
				}
			}
		}
	}()

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	select {
	case err := <-waitCh:
		<-done
		if err != nil {
			return fmt.Errorf("hls: ffmpeg execution failed: %w", err)
		}
	case <-dlCtx.Done():
		select {
		case err := <-waitCh:
			<-done
			if err != nil {
				return fmt.Errorf("hls: ffmpeg execution failed: %w", err)
			}
		case <-time.After(5 * time.Second):
			// Close stderr pipe to wake up the scanner goroutine
			if pipeErr := stderr.Close(); pipeErr != nil {
				slog.Warn("Failed to close stderr pipe during cancel timeout", logutil.LogKeyError, pipeErr)
			}
			// Wait for scanner with a short grace period; don't block forever
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				slog.Warn("Scanner goroutine did not exit after stderr close, possible leak",
					logutil.LogKeyURL, req.URL)
			}
			return fmt.Errorf("hls: ffmpeg cancel timeout")
		}
	case <-time.After(e.ffmpegTimeout):
		dlCancel() // 立即终止 ffmpeg 进程
		// Close stderr pipe to wake up the scanner goroutine on timeout
		if pipeErr := stderr.Close(); pipeErr != nil {
			slog.Warn("Failed to close stderr pipe during execution timeout", logutil.LogKeyError, pipeErr)
		}
		select {
		case err := <-waitCh:
			<-done
			if err != nil {
				slog.Warn("ffmpeg killed on timeout", logutil.LogKeyError, err)
			}
		case <-time.After(3 * time.Second):
			slog.Warn("Scanner goroutine did not exit after stderr close on timeout",
				logutil.LogKeyURL, req.URL)
		}
		return fmt.Errorf("hls: ffmpeg execution timeout")
	}

	reportHLSDownloadResult(rPath, req)
	return nil
}

func reportHLSDownloadResult(rPath string, req *download.Request) {
	var size int64
	if info, err := os.Stat(rPath); err == nil {
		size = info.Size()
		if req.Result == nil {
			req.Result = &download.DownloadResult{}
		}
		req.Result.ContentLength = size
	}
	if req.OnProgress != nil {
		req.OnProgress(100, size, size)
	}
}

// downloadWithM3U8D 使用纯 Go 的 M3U8DEngine 下载 HLS（无需 ffmpeg）。
// 流程：M3U8DEngine 解析主/子 m3u8 → 并发下载分片（grab）→ 按 m3u8 顺序拼接 .ts → 输出。
func (e *HLSExtractor) downloadWithM3U8D(ctx context.Context, req *download.Request) error {
	if err := validateHLSParams(req); err != nil {
		return err
	}
	rPath := req.SavePath
	if err := os.MkdirAll(filepath.Dir(rPath), 0o755); err != nil {
		return fmt.Errorf("hls: failed to create directory: %w", err)
	}

	// 工作目录：输出文件旁 .hls-parts
	workDir := rPath + ".hls-parts"
	cfg := &m3u8d.DownloadConfig{
		InputURL:    req.URL,
		OutputFile:  rPath,
		UserAgent:   e.userAgent,
		Headers:     req.Headers,
		Concurrency: 8,
		MaxRetries:  3,
		WorkDir:     workDir,
		MinFiles:    1, // 兼容单分片/极小列表
		Timeout:     30 * time.Second,
	}
	engine, err := m3u8d.NewM3U8DEngine(cfg, nil)
	if err != nil {
		return fmt.Errorf("hls: m3u8d engine init: %w", err)
	}
	defer func() {
		if cerr := engine.Cleanup(); cerr != nil {
			slog.Warn("hls: m3u8d cleanup failed", logutil.LogKeyError, cerr)
		}
	}()

	// 下载 m3u8 + 全部分片
	mainM3U8Path, err := engine.DownloadAll(ctx)
	if err != nil {
		return fmt.Errorf("hls: m3u8d download: %w", err)
	}

	// 按 m3u8 顺序拼接分片
	if err := ConcatM3U8Segments(mainM3U8Path, workDir, rPath); err != nil {
		return fmt.Errorf("hls: m3u8d concat: %w", err)
	}

	reportHLSDownloadResult(rPath, req)
	return nil
}

// ConcatM3U8Segments 按 m3u8 中出现顺序拼接分片文件（.ts/.jpeg 伪装分片均已由
// M3U8DEngine 改写为 workdir 下的本地文件）。导出供测试与复用。
func ConcatM3U8Segments(localM3U8Path, workDir, outPath string) error {
	// 递归处理：主列表（含 STREAM-INF 档位）→ 选最后一个（最高清）子列表拼接；
	// 单层列表直接按顺序拼分片。
	var subList string
	for _, line := range readM3U8Lines(localM3U8Path) {
		if strings.Contains(strings.ToLower(line), ".m3u8") {
			subList = line // 记录最后的子列表（档位递增）
		}
	}
	if subList != "" {
		// 主列表：递归到最高档子列表
		subPath := filepath.Join(workDir, filepath.Base(subList))
		if _, err := os.Stat(subPath); err != nil {
			return fmt.Errorf("sub playlist %s: %w", subPath, err)
		}
		return ConcatM3U8Segments(subPath, workDir, outPath)
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	var count int
	for _, line := range readM3U8Lines(localM3U8Path) {
		if line == "" || strings.HasPrefix(line, "#") || strings.Contains(line, ".m3u8") {
			continue
		}
		// 分片本地路径（M3U8DEngine 改写为 workdir 下 hash.ts）
		segPath := filepath.Join(workDir, filepath.Base(line))
		f, err := os.Open(segPath)
		if err != nil {
			return fmt.Errorf("segment %s: %w", segPath, err)
		}
		if _, err := io.Copy(out, f); err != nil {
			f.Close()
			return err
		}
		f.Close()
		count++
	}
	if count == 0 {
		return fmt.Errorf("no segments found in m3u8")
	}
	slog.Info("HLS m3u8d concat done", "segments", count, logutil.LogKeyURL, outPath)
	return nil
}

// readM3U8Lines 读取 m3u8 文件并返回非空行（trimmed）。
func readM3U8Lines(path string) []string {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(content), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
