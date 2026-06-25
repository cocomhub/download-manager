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
	"github.com/cocomhub/download-manager/pkg/logutil"
)

const logTimestampFmt = "20060102150405"

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
		slog.Error("Failed to create log directory", "dir", logDir, logutil.LogKeyError, err)
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
	filesVal, ok := obj.Extra["files"]
	if !ok || filesVal == nil {
		return d.downloadFile(obj, true, obj, headers)
	}

	fileList, err := parseCompositeFileList(filesVal)
	if err != nil {
		slog.Error("Composite download with unknown files metadata type", logutil.LogKeyType, fmt.Sprintf("%T", filesVal), logutil.LogKeyTaskID, obj.TaskID)
		return fmt.Errorf("%w. extra: %+v", err, obj.Extra)
	}
	if len(fileList) == 0 {
		return fmt.Errorf("composite download error: 'files' metadata found but empty or invalid format. extra: %+v", obj.Extra)
	}

	return d.downloadCompositeFiles(fileList, obj, headers)
}

var (
	reProgress = regexp.MustCompile(`\s+(\d+)%`)
)

func (d *WgetDownloader) downloadFile(subObj *model.DownloadObject, trackProgress bool, progressObj *model.DownloadObject, headers map[string]string) error {
	dir := filepath.Dir(subObj.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	logFile, logF := d.setupLogFile(subObj.SavePath)
	if logF != nil {
		defer logF.Close()
	}

	proxyURL := d.selectProxy(subObj.URL)
	args := buildWgetArgs(subObj.URL, subObj.SavePath, proxyURL, headers)

	cmd := exec.Command("wget", args...) //nolint:gosec // wget lookup via PATH is standard
	cmd.Env = os.Environ()
	d.active.Store(subObj.URL, cmd)

	slog.Info("Starting download", "downloader", "wget", logutil.LogKeyURL, subObj.URL, "path", subObj.SavePath, "wget_log", logFile)

	if err := d.runWgetAndTrack(cmd, subObj.URL, logF, trackProgress, progressObj); err != nil {
		return err
	}

	if trackProgress && progressObj != nil {
		progressObj.Progress = 100
	}
	return nil
}

func parseCompositeFileList(filesVal any) ([]map[string]string, error) {
	switch files := filesVal.(type) {
	case []map[string]string:
		return files, nil
	case []any:
		var fileList []map[string]string
		for _, f := range files {
			fm, ok := f.(map[string]any)
			if !ok {
				continue
			}
			m := extractStringMap(fm)
			if len(m) > 0 {
				fileList = append(fileList, m)
			}
		}
		return fileList, nil
	default:
		return nil, fmt.Errorf("composite download error: unknown 'files' metadata type")
	}
}

func extractStringMap(fm map[string]any) map[string]string {
	m := make(map[string]string, len(fm))
	for k, v := range fm {
		if s, ok := v.(string); ok {
			m[k] = s
		}
	}
	return m
}

func (d *WgetDownloader) downloadCompositeFiles(fileList []map[string]string, obj *model.DownloadObject, headers map[string]string) error {
	slog.Info("Starting composite download", "count", len(fileList), logutil.LogKeyTaskID, obj.TaskID)
	for _, fileMap := range fileList {
		url := fileMap["url"]
		path := fileMap["path"]
		fType := fileMap["type"]

		if url == "" || path == "" {
			continue
		}

		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory for composite file: %w", err)
		}

		subObj := &model.DownloadObject{
			URL:      url,
			SavePath: path,
		}

		trackProgress := (fType == "video" || len(fileList) == 1)

		if err := d.downloadFile(subObj, trackProgress, obj, headers); err != nil {
			return err
		}
	}

	obj.SetProgress(100)
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

// --- Helpers ---

func (d *WgetDownloader) setupLogFile(savePath string) (string, *os.File) {
	if d.logDir == "" {
		return "", nil
	}
	logFile := filepath.Join(d.logDir, filepath.Base(savePath)+"."+time.Now().Format(logTimestampFmt)+".wget.log")
	f, err := os.Create(logFile)
	if err != nil {
		slog.Warn("Failed to create wget log file", "file", logFile, logutil.LogKeyError, err)
		return "", nil
	}
	return logFile, f
}

func (d *WgetDownloader) selectProxy(targetURL string) string {
	if d.proxySelector == nil {
		return ""
	}
	proxyURL, err := d.determineProxy(targetURL)
	if err != nil {
		slog.Warn("Proxy selection failed, falling back to direct", logutil.LogKeyURL, targetURL, logutil.LogKeyError, err)
		return ""
	}
	return proxyURL
}

func buildWgetArgs(url, savePath, proxyURL string, headers map[string]string) []string {
	args := []string{"-c", "-T", "20", "-t", "5"}
	args = append(args, "--header", "User-Agent: "+DefaultUserAgent)
	for k, v := range headers {
		args = append(args, "--header", fmt.Sprintf("%s: %s", k, v))
	}

	effectiveURL := url
	if proxyURL != "" {
		effectiveURL = strings.TrimPrefix(url, "http://")
		effectiveURL = strings.TrimPrefix(effectiveURL, "https://")
		effectiveURL = proxyURL + "/" + effectiveURL
		slog.Info("Using proxy", logutil.LogKeyURL, effectiveURL, "proxy", proxyURL)
	} else {
		slog.Debug("Using direct connection", logutil.LogKeyURL, url)
	}

	args = append(args, "-O", savePath, effectiveURL)
	return args
}

func (d *WgetDownloader) runWgetAndTrack(cmd *exec.Cmd, url string, logF *os.File, trackProgress bool, progressObj *model.DownloadObject) error {
	stderr, err := cmd.StderrPipe()
	if err != nil {
		d.active.Delete(url)
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}
	cmd.Stdout = logF

	if err := cmd.Start(); err != nil {
		d.active.Delete(url)
		return fmt.Errorf("wget start failed: %w", err)
	}

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		if logF != nil {
			_, _ = logF.WriteString(line + "\n")
		}
		trackProgressLine(line, trackProgress, progressObj)
	}

	if err := cmd.Wait(); err != nil {
		d.active.Delete(url)
		return fmt.Errorf("wget execution failed: %w", err)
	}
	d.active.Delete(url)
	return nil
}

func trackProgressLine(line string, trackProgress bool, progressObj *model.DownloadObject) {
	if !trackProgress || progressObj == nil {
		return
	}
	matches := reProgress.FindStringSubmatch(line)
	if len(matches) > 1 {
		if p, err := strconv.Atoi(matches[1]); err == nil {
			progressObj.SetProgress(p)
		}
	}
}
