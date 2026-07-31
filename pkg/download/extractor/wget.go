// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

const DefaultWgetUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"

const logTimestampFmt = "20060102150405"

var reWgetProgress = regexp.MustCompile(`\s+(\d+)%`)

// compile-time interface check
var _ download.Extractor = (*WgetExtractor)(nil)
var _ download.Canceller = (*WgetExtractor)(nil)

// WgetExtractor 将 wget 命令行工具包装为 Extractor 接口。
// 不依赖 Transport，自己管理 exec.Command 来执行 wget 进程。
// Deprecated: WgetExtractor is not currently registered or used in production.
// Use HTTPExtractor instead. Retained for future reference.
type WgetExtractor struct {
	logDir      string
	selector    download.Selector
	active      sync.Map
	userAgent   string
	maxRetries  int
	timeoutSecs int
	// RedactSensitiveHeaders 控制是否在命令行参数中排除敏感头。默认 true。
	RedactSensitiveHeaders bool
}

// NewWgetExtractor 创建 WgetExtractor 实例。
// TODO: 预留扩展 - 当前未被自动匹配使用，通过 hint.Extractor == "wget" 显式选择。
// 后续可根据需求扩展自动匹配逻辑。
func NewWgetExtractor(opts ...WgetOption) *WgetExtractor {
	e := &WgetExtractor{
		userAgent:              DefaultWgetUserAgent,
		maxRetries:             5,
		timeoutSecs:            20,
		RedactSensitiveHeaders: true,
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// WgetOption 是 WgetExtractor 的配置函数。
type WgetOption func(*WgetExtractor)

// WithWgetLogDir 设置 wget 日志目录。
func WithWgetLogDir(dir string) WgetOption { return func(e *WgetExtractor) { e.logDir = dir } }

// WithWgetUserAgent 设置自定义 User-Agent。
func WithWgetUserAgent(ua string) WgetOption { return func(e *WgetExtractor) { e.userAgent = ua } }

// WithWgetMaxRetries 设置最大重试次数。
func WithWgetMaxRetries(n int) WgetOption { return func(e *WgetExtractor) { e.maxRetries = n } }

// WithWgetTimeout 设置下载超时秒数。
func WithWgetTimeout(secs int) WgetOption { return func(e *WgetExtractor) { e.timeoutSecs = secs } }

// WithWgetRedactSensitiveHeaders 设置敏感头过滤开关。
func WithWgetRedactSensitiveHeaders(v bool) WgetOption {
	return func(e *WgetExtractor) { e.RedactSensitiveHeaders = v }
}

func (e *WgetExtractor) Name() string { return "wget" }

// Match 返回 false：WgetExtractor 不参与自动 URL 匹配。
// 仅通过 hint.Extractor == "wget" 显式选择，不参与自动匹配。
func (e *WgetExtractor) Match(ctx context.Context, url string) bool {
	return false
}

// SetSelector 注入 Selector 实例用于代理选择。
func (e *WgetExtractor) SetSelector(s download.Selector) { e.selector = s }

// SetTransport is a no-op: wget does not use a Go Transport.
// Implemented for download.TransportSetter interface compatibility.
func (e *WgetExtractor) SetTransport(t download.Transport) {}

// Extract 使用 wget 命令行下载文件。
func (e *WgetExtractor) Extract(ctx context.Context, req *download.Request) error {
	dir := filepath.Dir(req.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("wget: failed to create directory: %w", err)
	}

	if err := validateWgetRequest(req); err != nil {
		return err
	}

	logFile := e.openWgetLogFile(req.SavePath)
	if logFile != nil {
		defer logFile.Close()
	}

	proxyURL := e.selectWgetProxy(ctx, req)
	args := e.buildWgetArgs(req, proxyURL)

	// Pre-check: verify wget is available in PATH before constructing the command.
	if _, err := exec.LookPath("wget"); err != nil {
		return fmt.Errorf("wget: not found in PATH: %w", err)
	}

	cmd := exec.CommandContext(ctx, "wget", args...) //nolint:gosec // wget lookup via PATH is standard

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("wget: failed to get stderr pipe: %w", err)
	}
	cmd.Stdout = logFile

	slog.Info("Starting download", "downloader", "wget", logutil.LogKeyURL, req.URL, "path", req.SavePath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("wget: start failed: %w", err)
	}

	// Store cmd in active map only after cmd.Start() succeeds,
	// to prevent Cancel from accessing cmd.Process before it's initialized.
	e.active.Store(req.URL, cmd)

	scanWgetProgress(stderr, logFile, req)

	if err := cmd.Wait(); err != nil {
		e.active.Delete(req.URL)
		return fmt.Errorf("wget: execution failed: %w", err)
	}
	e.active.Delete(req.URL)

	reportWgetFinalProgress(req)
	return nil
}

// validateWgetRequest checks for argv injection and invalid inputs.
func validateWgetRequest(req *download.Request) error {
	if strings.HasPrefix(req.SavePath, "-") {
		return fmt.Errorf("wget: invalid save path (starts with '-')")
	}
	if strings.ContainsAny(req.SavePath, "\r\n") {
		return fmt.Errorf("wget: invalid save path contains CR/LF")
	}
	for k, v := range req.Headers {
		if strings.ContainsAny(k, "\r\n") || strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("wget: invalid header contains CR/LF")
		}
		if strings.HasPrefix(k, "-") || strings.HasPrefix(v, "-") {
			return fmt.Errorf("wget: header key/value starts with '-', possible argv injection")
		}
	}
	lowerURL := strings.ToLower(req.URL)
	if !strings.HasPrefix(lowerURL, "http://") &&
		!strings.HasPrefix(lowerURL, "https://") &&
		!strings.HasPrefix(lowerURL, "ftp://") {
		return fmt.Errorf("wget: invalid URL scheme: %s", req.URL)
	}
	if strings.ContainsAny(req.URL, "\r\n") {
		return fmt.Errorf("wget: invalid URL contains CR/LF")
	}
	return nil
}

// openWgetLogFile opens a log file for wget output, or returns nil if logging is disabled.
func (e *WgetExtractor) openWgetLogFile(savePath string) *os.File {
	if e.logDir == "" {
		return nil
	}
	logFilePath := filepath.Join(e.logDir, filepath.Base(savePath)+"."+time.Now().Format(logTimestampFmt)+".wget.log")
	f, err := os.Create(logFilePath)
	if err != nil {
		slog.Warn("Failed to create wget log file", "file", logFilePath, logutil.LogKeyError, err)
		return nil
	}
	return f
}

// selectWgetProxy selects a proxy URL if a selector is configured.
func (e *WgetExtractor) selectWgetProxy(ctx context.Context, req *download.Request) string {
	if e.selector == nil {
		return ""
	}
	hint := req.Hint
	if hint == nil {
		hint = &download.DownloadHint{}
	}
	proxyURL, err := e.selector.SelectProxy(ctx, req.URL, hint)
	if err != nil {
		slog.Warn("Proxy selection failed, falling back to direct", logutil.LogKeyURL, req.URL, logutil.LogKeyError, err)
		return ""
	}
	return proxyURL
}

// buildWgetArgs constructs the wget command-line arguments.
func (e *WgetExtractor) buildWgetArgs(req *download.Request, proxyURL string) []string {
	args := []string{"-c", "-T", strconv.Itoa(e.timeoutSecs), "-t", strconv.Itoa(e.maxRetries)}
	args = append(args, "--header", "User-Agent: "+e.userAgent)

	for k, v := range req.Headers {
		if e.RedactSensitiveHeaders && download.IsSensitiveHeader(k) {
			continue
		}
		args = append(args, "--header", fmt.Sprintf("%s: %s", k, v))
	}

	targetURL := req.URL
	if proxyURL != "" {
		// 使用 url.URL 安全拼接代理 URL，避免字符串操作风险
		u, err := url.Parse(req.URL)
		if err != nil {
			slog.Warn("Failed to parse URL for proxy, fallback to raw", logutil.LogKeyURL, req.URL, logutil.LogKeyError, err)
			targetURL = proxyURL + "/" + req.URL
		} else {
			proxy, err := url.Parse(proxyURL)
			if err != nil {
				slog.Warn("Failed to parse proxy URL, fallback to raw", "proxy", proxyURL, logutil.LogKeyError, err)
				targetURL = proxyURL + "/" + req.URL
			} else {
				p := *proxy
				p.Path = proxy.Path + "/" + u.Host + u.Path
				p.RawQuery = u.RawQuery
				targetURL = p.String()
			}
		}
		slog.Info("Using proxy", logutil.LogKeyURL, targetURL, "proxy", proxyURL)
	}

	args = append(args, "-O", req.SavePath, targetURL)
	return args
}

// scanWgetProgress reads wget stderr to report download progress.
func scanWgetProgress(stderr io.Reader, logFile *os.File, req *download.Request) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		if logFile != nil {
			_, _ = logFile.WriteString(line + "\n")
		}
		if !req.TrackProgress || req.OnProgress == nil {
			continue
		}
		if progress, downloaded, ok := parseWgetProgressLine(line, req.SavePath); ok {
			req.OnProgress(progress, downloaded, 0)
		}
	}
}

// parseWgetProgressLine parses a wget progress line and returns the percentage,
// downloaded bytes, and whether parsing succeeded.
func parseWgetProgressLine(line string, savePath string) (progress float64, downloaded int64, ok bool) {
	matches := reWgetProgress.FindStringSubmatch(line)
	if len(matches) <= 1 {
		return 0, 0, false
	}
	p, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, false
	}
	var size int64
	if info, statErr := os.Stat(savePath); statErr == nil {
		size = info.Size()
	}
	return float64(p), size, true
}

// reportWgetFinalProgress reports 100% completion after a successful download.
func reportWgetFinalProgress(req *download.Request) {
	if req.OnProgress == nil {
		return
	}
	var size int64
	if info, err := os.Stat(req.SavePath); err == nil {
		size = info.Size()
	}
	req.OnProgress(100, size, size)
}

// Cancel 取消正在进行的 wget 下载。
func (e *WgetExtractor) Cancel(url string) error {
	if v, ok := e.active.Load(url); ok {
		cmd, ok := v.(*exec.Cmd)
		if !ok {
			return fmt.Errorf("wget: unexpected type %T in active map", v)
		}
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		e.active.Delete(url)
		return nil
	}
	return nil
}
