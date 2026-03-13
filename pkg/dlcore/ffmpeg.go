// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dlcore

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func isHlsURL(u string) bool {
	lu := strings.ToLower(u)
	return strings.Contains(lu, ".m3u8")
}

func (c *Client) downloadHLSWithFFmpeg(ctx context.Context, req *Request) error {
	slog.Info("hls_auto_mark_as_fail", "status", c.hlsAutoMarkAsFail)
	if c.hlsAutoMarkAsFail {
		return fmt.Errorf("hls auto mark as fail: %w", ErrNoTry)
	}
	dir := filepath.Dir(req.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	var logFile string
	var f *os.File
	if c.logDir != "" {
		logFile = filepath.Join(c.logDir, filepath.Base(req.SavePath)+"."+time.Now().Format("20060102150405")+".ffmpeg.log")
		var err error
		f, err = os.Create(logFile)
		if err != nil {
			slog.Warn("Failed to create ffmpeg log file", "file", logFile, "error", err)
		} else {
			defer f.Close()
		}
	}
	ffmpeg := c.ffmpegPath
	if strings.TrimSpace(ffmpeg) == "" {
		ffmpeg = "ffmpeg"
	}
	if path, err := exec.LookPath(ffmpeg); err == nil {
		ffmpeg = path
	} else {
		return fmt.Errorf("ffmpeg not found: please install ffmpeg or set downloader.ffmpeg_path")
	}
	args := []string{"-y"}
	if ua := DefaultUserAgent; ua != "" {
		args = append(args, "-user_agent", ua)
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
	args = append(args, "-c", "copy", "-bsf:a", "aac_adtstoasc", "-movflags", "+faststart", "-f", "mp4", req.SavePath)

	dctx, cancel := context.WithCancel(ctx)
	c.active.Store(req.URL, cancel)
	defer c.active.Delete(req.URL)
	cmd := exec.CommandContext(dctx, ffmpeg, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg: failed to attach stderr: %w", err)
	}
	cmd.Stdout = f
	slog.Info("Starting download", "downloader", "ffmpeg", "url", req.URL, "path", req.SavePath, "ffmpeg_log", logFile)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start failed: %w", err)
	}
	go func() {
		io.Copy(f, stderr)
	}()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg execution failed: %w", err)
	}
	if req.TrackProgress && req.OnProgress != nil {
		req.OnProgress(100, 0, 0)
	}
	req.Metadata["status"] = StatusCompleted
	if info, err := os.Stat(req.SavePath); err == nil {
		req.Metadata["total_size"] = strconv.FormatInt(info.Size(), 10)
	}
	return nil
}
