// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

var (
	DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
)

type NativeHTTPDownloader struct {
	logDir            string
	proxies           []string
	cacheFile         string
	forceProxy        bool
	maxRetries        int
	client            *http.Client
	dLimiter          *DomainLimiter
	active            sync.Map
	ffmpegPath        string
	hlsAutoMarkAsFail bool
	coreClient        *dlcore.Client
}

var _ core.Downloader = &NativeHTTPDownloader{}

func NewNativeHTTPDownloader(cfg config.Downloader) *NativeHTTPDownloader {
	logDir := cfg.LogDir
	if err := os.MkdirAll(logDir, 0755); err != nil {
		slog.Error("Failed to create log directory", "dir", logDir, "error", err)
		logDir = ""
	}

	home, _ := os.UserHomeDir()

	// 创建配置化的 HTTP 客户端 [1,2](@ref)
	client := &http.Client{
		Timeout: 600 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	coreClient := dlcore.NewClient(
		dlcore.WithHTTPClient(client),
		dlcore.WithLoggerDir(logDir),
		dlcore.WithProxies(cfg.Proxies),
		dlcore.WithForceProxy(cfg.ForceProxy),
		dlcore.WithMaxRetries(cfg.MaxRetries),
		dlcore.WithFFmpegPath(cfg.FfmpegPath),
		dlcore.WithHLSAutoMarkAsFail(cfg.HlsAutoMarkAsFail),
	)

	return &NativeHTTPDownloader{
		logDir:            logDir,
		proxies:           cfg.Proxies,
		cacheFile:         filepath.Join(home, ".config/download-manager/proxy_cache"),
		forceProxy:        cfg.ForceProxy,
		maxRetries:        cfg.MaxRetries,
		client:            client,
		dLimiter:          NewDomainLimiter(),
		ffmpegPath:        cfg.FfmpegPath,
		hlsAutoMarkAsFail: cfg.HlsAutoMarkAsFail,
		coreClient:        coreClient,
	}
}

func (d *NativeHTTPDownloader) ApplyDomainLimits(limits map[string]int) {
	for host, max := range limits {
		d.dLimiter.Set(host, max)
	}
	if d.coreClient != nil {
		d.coreClient.ApplyDomainLimits(limits)
	}
}

func (d *NativeHTTPDownloader) Name() string {
	return "native_http"
}

func (d *NativeHTTPDownloader) Download(obj *model.DownloadObject, headers map[string]string) error {
	// 复合下载逻辑保持不变
	if filesVal, ok := obj.Extra["files"]; ok && filesVal != nil {
		var fileList []map[string]string

		if files, ok := filesVal.(primitive.A); ok {
			filesVal = []any(files)
		}

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
			slog.Error("Composite download with unknown files metadata type",
				"type", fmt.Sprintf("%T", filesVal), "task_id", obj.TaskID)
			return fmt.Errorf("composite download error: unknown 'files' metadata type")
		}

		if len(fileList) == 0 {
			return fmt.Errorf("composite download error: 'files' metadata found but empty")
		}

		slog.Info("Starting composite download", "count", len(fileList), "task_id", obj.TaskID)
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

			trackProgress := (fType == "video" || len(fileList) == 1)

			req := &dlcore.Request{
				URL:           url,
				SavePath:      path,
				Headers:       headers,
				TrackProgress: trackProgress,
				OnProgress: func(p float64, downloaded, total int64) {
					obj.Progress = int(p)
				},
				Metadata: fileMap,
			}
			if err := d.coreClient.Download(context.Background(), req); err != nil {
				return err
			}
		}
		obj.Progress = 100
		return nil
	}

	// 单文件下载
	req := &dlcore.Request{
		URL:           obj.URL,
		SavePath:      obj.SavePath,
		Headers:       headers,
		TrackProgress: true,
		OnProgress: func(p float64, downloaded, total int64) {
			obj.Progress = int(p)
		},
		Metadata: obj.Metadata,
	}
	if err := d.coreClient.Download(context.Background(), req); err != nil {
		return err
	}
	return nil
}

var (
	ErrNoTry = errors.New("no try left")
)

func (d *NativeHTTPDownloader) Cancel(url string) error {
	if d.coreClient != nil {
		return d.coreClient.Cancel(url)
	}
	return fmt.Errorf("core client not init")
}
