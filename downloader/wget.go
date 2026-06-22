// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

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

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/download"
)

type WgetDownloader struct {
	logDir        string
	proxies       []string
	cacheDir      string
	forceProxy    bool
	active        sync.Map
	proxySelector *download.StaticProxySelector
}

// Ensure WgetDownloader implements core.Downloader
var _ core.Downloader = &WgetDownloader{}

func NewWgetDownloader(cfg config.Downloader) *WgetDownloader {
	logDir := cfg.LogDir
	if err := os.MkdirAll(logDir, 0755); err != nil {
		slog.Error("Failed to create log directory", "dir", logDir, "error", err)
		logDir = ""
	}

	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".config/download-manager/proxy_cache")

	return &WgetDownloader{
		logDir:     logDir,
		proxies:    cfg.Proxies,
		cacheDir:   cacheDir,
		forceProxy: cfg.ForceProxy,
		proxySelector: download.NewStaticProxySelector(cfg.Proxies).
			WithForceProxy(cfg.ForceProxy).
			WithCache(cacheDir, 1).
			WithProbe(3),
	}
}

func (d *WgetDownloader) Name() string {
	return "wget"
}

func (d *WgetDownloader) Download(obj *model.DownloadObject, headers map[string]string) error {
	// Check for composite download (files in Extra)
	if filesVal, ok := obj.Extra["files"]; ok && filesVal != nil {
		var fileList []map[string]string

		// Handle different types depending on source (memory vs JSON)
		if files, ok := filesVal.([]map[string]string); ok {
			fileList = files
		} else if files, ok := filesVal.([]any); ok {
			for _, f := range files {
				if fm, ok := f.(map[string]any); ok {
					m := make(map[string]string)
					for k, v := range fm {
						if s, ok := v.(string); ok {
							m[k] = s
						}
					}
					fileList = append(fileList, m)
				}
			}
		} else {
			slog.Error("Composite download with unknown files metadata type", "type", fmt.Sprintf("%T", filesVal), "task_id", obj.TaskID)
			return fmt.Errorf("composite download error: unknown 'files' metadata type. extra: %+v", obj.Extra)
		}

		if len(fileList) == 0 {
			// If "files" key exists but we couldn't parse it or it's empty,
			// it's a data integrity issue. Do NOT fallback to single file download
			// because obj.URL might be a web page, not a file.
			return fmt.Errorf("composite download error: 'files' metadata found but empty or invalid format. extra: %+v", obj.Extra)
		}

		if len(fileList) > 0 {
			slog.Info("Starting composite download", "count", len(fileList), "task_id", obj.TaskID)
			for _, fileMap := range fileList {
				url := fileMap["url"]
				path := fileMap["path"]
				fType := fileMap["type"]

				if url == "" || path == "" {
					continue
				}

				// Create directory for this file
				dir := filepath.Dir(path)
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("failed to create directory for composite file: %w", err)
				}

				// Construct a temporary object for this file
				subObj := &model.DownloadObject{
					URL:      url,
					SavePath: path,
				}

				// Track progress only for video (main content), or if it's the only file
				trackProgress := (fType == "video" || len(fileList) == 1)

				if err := d.downloadFile(subObj, trackProgress, obj, headers); err != nil {
					return err
				}
			}
			// Ensure 100% at the end
			obj.SetProgress(100)
			return nil
		}
	}

	// Fallback to standard single file download
	return d.downloadFile(obj, true, obj, headers)
}

var (
	reProgress = regexp.MustCompile(`\s+(\d+)%`)
)

func (d *WgetDownloader) downloadFile(subObj *model.DownloadObject, trackProgress bool, progressObj *model.DownloadObject, headers map[string]string) error {
	// Ensure directory exists
	dir := filepath.Dir(subObj.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Prepare log file for wget output
	var f *os.File
	var logFile string
	if d.logDir != "" {
		logFile = filepath.Join(d.logDir, filepath.Base(subObj.SavePath)+"."+time.Now().Format("20060102150405")+".wget.log")
		var err error
		f, err = os.Create(logFile)
		if err != nil {
			slog.Warn("Failed to create wget log file", "file", logFile, "error", err)
		} else {
			defer f.Close()
		}
	}

	// Determine connection mode (direct or proxy)
	proxyURL := ""
	if d.proxySelector != nil {
		var err error
		proxyURL, err = d.determineProxy(subObj.URL)
		if err != nil {
			slog.Warn("Proxy selection failed, falling back to direct", "url", subObj.URL, "error", err)
		}
	}

	// Build wget command
	args := []string{"-c", "-T", "20", "-t", "5"}

	// Add User-Agent
	args = append(args, "--header", "User-Agent: "+DefaultUserAgent)

	// Add custom headers
	for k, v := range headers {
		args = append(args, "--header", fmt.Sprintf("%s: %s", k, v))
	}

	url := subObj.URL
	// Add proxy arguments if selected
	env := os.Environ()
	if proxyURL != "" {
		url = strings.TrimPrefix(url, "http://")
		url = strings.TrimPrefix(url, "https://")
		url = proxyURL + "/" + url
		slog.Info("Using proxy", "url", url, "proxy", proxyURL)
		// Set both environment variables and command line args to be safe,
		// but typically environment variables are enough for wget if not using --no-proxy
		// Using -e http_proxy=... works well with wget
		// args = append(args, "-e", "use_proxy=yes")
		// args = append(args, "-e", "http_proxy="+proxyURL)
		// args = append(args, "-e", "https_proxy="+proxyURL)
	} else {
		slog.Debug("Using direct connection", "url", url)
	}

	args = append(args, "-O", subObj.SavePath, url)

	cmd := exec.Command("wget", args...) //nolint:gosec // wget lookup via PATH is standard
	cmd.Env = env
	d.active.Store(subObj.URL, cmd)

	// Wget writes progress to stderr
	stderr, err := cmd.StderrPipe()
	if err != nil {
		d.active.Delete(subObj.URL)
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Also capture stdout just in case
	cmd.Stdout = f

	slog.Info("Starting download", "downloader", "wget", "url", subObj.URL, "path", subObj.SavePath, "wget_log", logFile)

	if err := cmd.Start(); err != nil {
		d.active.Delete(subObj.URL)
		return fmt.Errorf("wget start failed: %w", err)
	}

	// Parse progress from stderr
	scanner := bufio.NewScanner(stderr)

	for scanner.Scan() {
		line := scanner.Text()

		// Write to log file
		if f != nil {
			_, _ = f.WriteString(line + "\n")
		}

		if trackProgress && progressObj != nil {
			// Extract progress
			matches := reProgress.FindStringSubmatch(line)
			if len(matches) > 1 {
				if p, err := strconv.Atoi(matches[1]); err == nil {
					progressObj.SetProgress(p)
				}
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		d.active.Delete(subObj.URL)
		return fmt.Errorf("wget execution failed: %w", err)
	}
	d.active.Delete(subObj.URL)

	if trackProgress && progressObj != nil {
		progressObj.Progress = 100
	}

	return nil
}

func (d *WgetDownloader) Cancel(url string) error {
	if v, ok := d.active.Load(url); ok {
		cmd := v.(*exec.Cmd)
		_ = cmd.Process.Kill()
		d.active.Delete(url)
		return nil
	}
	return fmt.Errorf("no active download for url")
}

// --- Proxy Logic ---

func (d *WgetDownloader) determineProxy(targetURL string) (string, error) {
	return d.proxySelector.Select(context.Background(), targetURL, nil)
}
