// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
)

const DefaultWgetUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"

var reWgetProgress = regexp.MustCompile(`\s+(\d+)%`)

// compile-time interface check
var _ download.Extractor = (*WgetExtractor)(nil)

// WgetExtractor 将 wget 命令行工具包装为 Extractor 接口。
// 不依赖 Transport，自己管理 exec.Command 来执行 wget 进程。
type WgetExtractor struct {
	logDir      string
	selector    download.Selector
	active      sync.Map
	userAgent   string
	maxRetries  int
	timeoutSecs int
}

// NewWgetExtractor 创建 WgetExtractor 实例。
func NewWgetExtractor(opts ...WgetOption) *WgetExtractor {
	e := &WgetExtractor{
		userAgent:   DefaultWgetUserAgent,
		maxRetries:  5,
		timeoutSecs: 20,
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

func (e *WgetExtractor) Name() string { return "wget" }

// Match 匹配非 m3u8 URL，与 HTTPExtractor 互补。
func (e *WgetExtractor) Match(ctx context.Context, url string) bool {
	return !strings.Contains(strings.ToLower(url), ".m3u8")
}

// SetSelector 注入 Selector 实例用于代理选择。
func (e *WgetExtractor) SetSelector(s download.Selector) { e.selector = s }

// SetTransport 是空操作，wget 不依赖 Transport。
func (e *WgetExtractor) SetTransport(t download.Transport) {}

// Extract 使用 wget 命令行下载文件。
func (e *WgetExtractor) Extract(ctx context.Context, req *download.Request) error {
	dir := filepath.Dir(req.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("wget: failed to create directory: %w", err)
	}

	// Validate arguments to prevent argv injection
	if strings.HasPrefix(req.SavePath, "-") {
		return fmt.Errorf("wget: invalid save path (starts with '-')")
	}
	for k, v := range req.Headers {
		if strings.ContainsAny(k, "\r\n") || strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("wget: invalid header contains CR/LF")
		}
	}
	if !strings.HasPrefix(strings.ToLower(req.URL), "http://") &&
		!strings.HasPrefix(strings.ToLower(req.URL), "https://") &&
		!strings.HasPrefix(strings.ToLower(req.URL), "ftp://") {
		return fmt.Errorf("wget: invalid URL scheme: %s", req.URL)
	}

	var f *os.File
	if e.logDir != "" {
		logFile := filepath.Join(e.logDir, filepath.Base(req.SavePath)+"."+time.Now().Format("20060102150405")+".wget.log")
		var err error
		f, err = os.Create(logFile)
		if err != nil {
			slog.Warn("Failed to create wget log file", "file", logFile, "error", err)
		} else {
			defer f.Close()
		}
	}

	proxyURL := ""
	if e.selector != nil {
		var err error
		proxyURL, err = e.selector.SelectProxy(ctx, req.URL, req.Hint)
		if err != nil {
			slog.Warn("Proxy selection failed, falling back to direct", "url", req.URL, "error", err)
		}
	}

	args := []string{"-c", "-T", strconv.Itoa(e.timeoutSecs), "-t", strconv.Itoa(e.maxRetries)}
	args = append(args, "--header", "User-Agent: "+e.userAgent)

	for k, v := range req.Headers {
		args = append(args, "--header", fmt.Sprintf("%s: %s", k, v))
	}

	targetURL := req.URL
	if proxyURL != "" {
		targetURL = strings.TrimPrefix(targetURL, "http://")
		targetURL = strings.TrimPrefix(targetURL, "https://")
		targetURL = proxyURL + "/" + targetURL
		slog.Info("Using proxy", "url", targetURL, "proxy", proxyURL)
	}

	args = append(args, "-O", req.SavePath, targetURL)

	cmd := exec.CommandContext(ctx, "wget", args...)
	e.active.Store(req.URL, cmd)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		e.active.Delete(req.URL)
		return fmt.Errorf("wget: failed to get stderr pipe: %w", err)
	}
	cmd.Stdout = f

	slog.Info("Starting download", "downloader", "wget", "url", req.URL, "path", req.SavePath)
	if err := cmd.Start(); err != nil {
		e.active.Delete(req.URL)
		return fmt.Errorf("wget: start failed: %w", err)
	}

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		if f != nil {
			_, _ = f.WriteString(line + "\n")
		}
		if req.TrackProgress && req.OnProgress != nil {
			if matches := reWgetProgress.FindStringSubmatch(line); len(matches) > 1 {
				if p, err := strconv.Atoi(matches[1]); err == nil {
					req.OnProgress(float64(p), 0, 0)
				}
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		e.active.Delete(req.URL)
		return fmt.Errorf("wget: execution failed: %w", err)
	}
	e.active.Delete(req.URL)

	if req.OnProgress != nil {
		req.OnProgress(100, 0, 0)
	}
	return nil
}

// Cancel 取消正在进行的 wget 下载。
func (e *WgetExtractor) Cancel(url string) error {
	if v, ok := e.active.Load(url); ok {
		cmd := v.(*exec.Cmd)
		_ = cmd.Process.Kill()
		e.active.Delete(url)
		return nil
	}
	return nil
}
