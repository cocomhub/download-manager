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
	"sync"
	"time"
)

func isHlsURL(u string) bool {
	lu := strings.ToLower(u)
	return strings.Contains(lu, ".m3u8")
}

var (
	hlsData sync.Map
)

func (c *Client) downloadHLSWithFFmpeg(ctx context.Context, req *Request) error {
	slog.Info("hls_auto_mark_as_fail", "status", c.hlsAutoMarkAsFail)
	if c.hlsAutoMarkAsFail {
		return fmt.Errorf("hls auto mark as fail: %w", ErrNoTry)
	}
	rPath := req.SavePath
	if c.rootDir != "" {
		if p, err := ResolvePath(c.rootDir, req.SavePath); err == nil {
			rPath = p
		} else {
			return err
		}
	}
	dir := filepath.Dir(rPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	var logFile string
	var f *os.File
	if c.logDir != "" {
		logDir := c.logDir
		logFile = filepath.Join(logDir, filepath.Base(rPath)+"."+time.Now().Format("20060102150405")+".ffmpeg.log")
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
	ua := c.defaultUserAgent
	if strings.TrimSpace(ua) == "" {
		ua = DefaultUserAgent
	}
	if ua != "" {
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
	newSavePath := filepath.Base(req.SavePath)
	if strings.Contains(req.SavePath, "] ") {
		newSavePath = newSavePath[1:strings.Index(newSavePath, "] ")] + newSavePath[strings.LastIndex(newSavePath, "."):]
		slog.Info("newSavePath", "origin", req.SavePath, "newSavePath", newSavePath)
	}
	// core transcode/mux settings
	args = append(args, "-c", "copy", "-bsf:a", "aac_adtstoasc", "-movflags", "+faststart", "-f", "mp4")
	// append extra ffmpeg args before output file path
	if len(c.ffmpegExtraArgs) > 0 {
		args = append(args, c.ffmpegExtraArgs...)
	}
	args = append(args, rPath)

	// optional move-if-exists and external bootstrap log
	if !c.hlsAutoMarkAsFail {
		if c.moveIfExistsEnabled && strings.TrimSpace(c.moveIfExistsDir) != "" {
			candidate := filepath.Join(c.moveIfExistsDir, newSavePath)
			if c.rootDir != "" {
				if p, err := ResolvePath(c.rootDir, candidate); err == nil {
					candidate = p
				} else {
					slog.Warn("Failed to resolve move-if-exists path", "path", candidate, "error", err)
				}
			}
			if _, err := os.Stat(candidate); err == nil {
				slog.Info("file already exists, moving into place", "path", candidate)
				if err := os.Rename(candidate, rPath); err != nil {
					return fmt.Errorf("hls auto mark as fail: %w", ErrNoTry)
				}
				if req.TrackProgress && req.OnProgress != nil {
					req.OnProgress(100, 0, 0)
				}
				req.Metadata["status"] = StatusCompleted
				if info, err := os.Stat(rPath); err == nil {
					req.Metadata["total_size"] = strconv.FormatInt(info.Size(), 10)
				}
				return nil
			}
		}
		if c.externalHLSLogEnabled && strings.TrimSpace(c.externalHLSLogPath) != "" {
			logPath := c.externalHLSLogPath
			if c.rootDir != "" {
				if p, err := ResolvePath(c.rootDir, logPath); err == nil {
					logPath = p
				} else {
					slog.Warn("Failed to resolve external HLS log path", "path", logPath, "error", err)
				}
			}
			hlsFile, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if hlsFile != nil {
				defer hlsFile.Close()
				fmt.Fprintf(hlsFile, "m3u8d -o '%s' -i '%s'\n", req.URL, newSavePath)
			}
			return fmt.Errorf("hls auto mark as fail: %w", ErrNoTry)
		}
	}

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
	if info, err := os.Stat(rPath); err == nil {
		req.Metadata["total_size"] = strconv.FormatInt(info.Size(), 10)
	}
	return nil
}
