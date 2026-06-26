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

	"github.com/cocomhub/download-manager/pkg/logutil"
)

func isHlsURL(u string) bool {
	lu := strings.ToLower(u)
	return strings.Contains(lu, ".m3u8")
}

func isImageURL(u string) bool {
	lower := strings.ToLower(u)
	extensions := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp"}
	for _, ext := range extensions {
		if strings.Contains(lower, ext) {
			return true
		}
	}
	return false
}

func (c *Client) downloadHLSWithFFmpeg(ctx context.Context, req *Request) error {
	slog.Info("hls_auto_mark_as_fail", logutil.LogKeyStatus, c.hlsAutoMarkAsFail)
	if c.hlsAutoMarkAsFail {
		return fmt.Errorf("hls auto mark as fail (path check): %w", ErrNoTry)
	}

	rPath, err := c.resolveSavePath(req)
	if err != nil {
		return err
	}

	logFile, logWriter := c.createFFmpegLogFile(rPath)
	if logWriter != nil {
		defer logWriter.Close()
	}

	ffmpegPath, err := c.findFFmpegPath()
	if err != nil {
		return err
	}

	args, newSavePath := c.buildFFmpegArgs(req, rPath)

	if done, err := c.tryMoveIfExists(req, rPath, newSavePath); done {
		return err
	}

	if err := c.tryExternalHSLLog(req, newSavePath); err != nil {
		return err
	}

	return c.executeFFmpeg(ctx, req, ffmpegPath, args, rPath, logFile, logWriter)
}

// resolveSavePath resolves the save path relative to rootDir and creates the target directory.
func (c *Client) resolveSavePath(req *Request) (string, error) {
	rPath := req.SavePath
	if c.rootDir != "" {
		p, err := ResolvePath(c.rootDir, req.SavePath)
		if err != nil {
			return "", err
		}
		rPath = p
	}
	dir := filepath.Dir(rPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	return rPath, nil
}

// createFFmpegLogFile creates the ffmpeg log file if logDir is configured.
func (c *Client) createFFmpegLogFile(rPath string) (string, *os.File) {
	if c.logDir == "" {
		return "", nil
	}
	logFileName := filepath.Base(rPath)
	if strings.HasPrefix(logFileName, "0") {
		parts := strings.Split(rPath, "/")
		if len(parts) > 2 {
			logFileName = parts[len(parts)-2] + " -- " + parts[len(parts)-1]
		}
	}
	logFile := filepath.Join(c.logDir, logFileName+"."+time.Now().Format(logTimestampFmt)+".ffmpeg.log")
	f, err := os.Create(logFile)
	if err != nil {
		slog.Warn("Failed to create ffmpeg log file", "file", logFile, logutil.LogKeyError, err)
		return logFile, nil
	}
	return logFile, f
}

// findFFmpegPath locates the ffmpeg executable.
func (c *Client) findFFmpegPath() (string, error) {
	ffmpeg := c.ffmpegPath
	if strings.TrimSpace(ffmpeg) == "" {
		ffmpeg = "ffmpeg"
	}
	path, err := exec.LookPath(ffmpeg)
	if err != nil {
		return "", fmt.Errorf("ffmpeg not found: please install ffmpeg or set downloader.ffmpeg_path")
	}
	return path, nil
}

// buildFFmpegArgs constructs the ffmpeg command-line arguments.
func (c *Client) buildFFmpegArgs(req *Request, rPath string) (args []string, newSavePath string) {
	args = []string{"-y"}

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

	newSavePath = computeOutputFilename(req)

	args = append(args, "-c", "copy", "-bsf:a", "aac_adtstoasc", "-movflags", "+faststart", "-f", "mp4")
	if len(c.ffmpegExtraArgs) > 0 {
		args = append(args, c.ffmpegExtraArgs...)
	}
	args = append(args, rPath)
	return args, newSavePath
}

// computeOutputFilename computes the output filename from the save path.
func computeOutputFilename(req *Request) string {
	name := filepath.Base(req.SavePath)
	if strings.Contains(req.SavePath, "] ") {
		name = name[1:strings.Index(name, "] ")] + name[strings.LastIndex(name, "."):]
		slog.Info("newSavePath", "origin", req.SavePath, "newSavePath", name)
	}
	return name
}

// tryMoveIfExists attempts the move-if-exists optimization.
// Returns (true, nil) on success, (true, error) on failure, (false, nil) if not applicable.
func (c *Client) tryMoveIfExists(req *Request, rPath, newSavePath string) (bool, error) {
	if !c.moveIfExistsEnabled || strings.TrimSpace(c.moveIfExistsDir) == "" {
		return false, nil
	}

	candidate := filepath.Join(c.moveIfExistsDir, newSavePath)
	if c.rootDir != "" {
		p, err := ResolvePath(c.rootDir, candidate)
		if err != nil {
			slog.Warn("Failed to resolve move-if-exists path", "path", candidate, logutil.LogKeyError, err)
			return false, nil
		}
		candidate = p
	}

	if _, err := os.Stat(candidate); err != nil {
		return false, nil
	}

	slog.Info("file already exists, moving into place", "path", candidate)
	if err := os.Rename(candidate, rPath); err != nil {
		return true, fmt.Errorf("hls auto mark as fail (process error): %w", ErrNoTry)
	}

	if req.TrackProgress && req.OnProgress != nil {
		req.OnProgress(100, 0, 0)
	}
	req.Metadata["status"] = StatusCompleted
	if info, err := os.Stat(rPath); err == nil {
		req.Metadata["total_size"] = strconv.FormatInt(info.Size(), 10)
	}
	return true, nil
}

// tryExternalHSLLog writes an external HLS bootstrap log and returns ErrNoTry.
func (c *Client) tryExternalHSLLog(req *Request, newSavePath string) error {
	if !c.externalHLSLogEnabled || strings.TrimSpace(c.externalHLSLogPath) == "" {
		return nil
	}

	logPath := c.externalHLSLogPath
	if c.rootDir != "" {
		if p, err := ResolvePath(c.rootDir, logPath); err == nil {
			logPath = p
		} else {
			slog.Warn("Failed to resolve external HLS log path", "path", logPath, logutil.LogKeyError, err)
		}
	}
	hlsFile, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if hlsFile != nil {
		defer hlsFile.Close()
		fmt.Fprintf(hlsFile, "m3u8d -o '%s' -i '%s'\n", req.URL, newSavePath)
	}
	return fmt.Errorf("hls auto mark as fail (exit code): %w", ErrNoTry)
}

// executeFFmpeg runs the ffmpeg command and handles post-processing.
func (c *Client) executeFFmpeg(ctx context.Context, req *Request, ffmpegPath string, args []string, rPath, logFile string, logWriter *os.File) error {
	dctx, cancel := context.WithCancel(ctx)
	c.active.Store(req.URL, cancel)
	defer c.active.Delete(req.URL)

	cmd := exec.CommandContext(dctx, ffmpegPath, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg: failed to attach stderr: %w", err)
	}
	cmd.Stdout = logWriter
	slog.Info("Starting download", "downloader", "ffmpeg", logutil.LogKeyURL, req.URL, "path", req.SavePath, "ffmpeg_log", logFile)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start failed: %w", err)
	}
	go func() {
		io.Copy(logWriter, stderr)
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
