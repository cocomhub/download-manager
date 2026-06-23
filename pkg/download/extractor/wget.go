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

const logTimestampFmt = "20060102150405"

const DefaultWgetUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"

var reWgetProgress = regexp.MustCompile(`\s+(\d+)%`)

// compile-time interface check
var _ download.Extractor = (*WgetExtractor)(nil)
var _ download.Canceller = (*WgetExtractor)(nil)

// WgetExtractor 灏?wget 鍛戒护琛屽伐鍏峰寘瑁呬负 Extractor 鎺ュ彛銆?// 涓嶄緷璧?Transport锛岃嚜宸辩鐞?exec.Command 鏉ユ墽琛?wget 杩涚▼銆?type WgetExtractor struct {
	logDir      string
	selector    download.Selector
	active      sync.Map
	userAgent   string
	maxRetries  int
	timeoutSecs int
}

// NewWgetExtractor 鍒涘缓 WgetExtractor 瀹炰緥銆?func NewWgetExtractor(opts ...WgetOption) *WgetExtractor {
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

// WgetOption 鏄?WgetExtractor 鐨勯厤缃嚱鏁般€?type WgetOption func(*WgetExtractor)

// WithWgetLogDir 璁剧疆 wget 鏃ュ織鐩綍銆?func WithWgetLogDir(dir string) WgetOption { return func(e *WgetExtractor) { e.logDir = dir } }

// WithWgetUserAgent 璁剧疆鑷畾涔?User-Agent銆?func WithWgetUserAgent(ua string) WgetOption { return func(e *WgetExtractor) { e.userAgent = ua } }

// WithWgetMaxRetries 璁剧疆鏈€澶ч噸璇曟鏁般€?func WithWgetMaxRetries(n int) WgetOption { return func(e *WgetExtractor) { e.maxRetries = n } }

// WithWgetTimeout 璁剧疆涓嬭浇瓒呮椂绉掓暟銆?func WithWgetTimeout(secs int) WgetOption { return func(e *WgetExtractor) { e.timeoutSecs = secs } }

func (e *WgetExtractor) Name() string { return "wget" }

// Match 鍖归厤闈?m3u8 URL锛屼笌 HTTPExtractor 浜掕ˉ銆?func (e *WgetExtractor) Match(ctx context.Context, url string) bool {
	return !strings.Contains(strings.ToLower(url), ".m3u8")
}

// SetSelector 娉ㄥ叆 Selector 瀹炰緥鐢ㄤ簬浠ｇ悊閫夋嫨銆?func (e *WgetExtractor) SetSelector(s download.Selector) { e.selector = s }

// SetTransport is a no-op: wget does not use a Go Transport.
// Implemented for download.TransportSetter interface compatibility.
func (e *WgetExtractor) SetTransport(t download.Transport) {}

// Extract 浣跨敤 wget 鍛戒护琛屼笅杞芥枃浠躲€?func (e *WgetExtractor) Extract(ctx context.Context, req *download.Request) error {
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
		logFile := filepath.Join(e.logDir, filepath.Base(req.SavePath)+"."+time.Now().Format(logTimestampFmt)+".wget.log")
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

	cmd := exec.CommandContext(ctx, "wget", args...) //nolint:gosec // wget lookup via PATH is standard
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
					// 鐢?os.Stat 鑾峰彇褰撳墠宸蹭笅杞藉瓧鑺傛暟浣滀负 downloaded
					var downloaded int64
					if info, statErr := os.Stat(req.SavePath); statErr == nil {
						downloaded = info.Size()
					}
					req.OnProgress(float64(p), downloaded, 0)
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
		// wget 閫氳繃 exec 涓嬭浇锛屽畬鎴愬悗鐢ㄦ枃浠跺疄闄呭ぇ灏忓～鍏?downloaded 涓?total銆?		var size int64
		if info, err := os.Stat(req.SavePath); err == nil {
			size = info.Size()
		}
		req.OnProgress(100, size, size)
	}
	return nil
}

// Cancel 鍙栨秷姝ｅ湪杩涜鐨?wget 涓嬭浇銆?func (e *WgetExtractor) Cancel(url string) error {
	if v, ok := e.active.Load(url); ok {
		cmd, ok := v.(*exec.Cmd)
		if !ok {
			return fmt.Errorf("wget: unexpected type %T in active map", v)
		}
		_ = cmd.Process.Kill()
		e.active.Delete(url)
		return nil
	}
	return nil
}
