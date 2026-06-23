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
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore" //nolint:staticcheck // deprecated dlcore used only by native_old downloader
)

var (
	DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
)

// Deprecated: NativeHTTPDownloader uses the deprecated pkg/dlcore.
// Use New() with a "native" type to get the new pkg/download path.
type NativeHTTPDownloader struct {
	ctx               context.Context
	logDir            string
	proxies           []string
	cacheFile         string
	forceProxy        bool
	maxRetries        int
	client            *http.Client
	ffmpegPath        string
	hlsAutoMarkAsFail bool
	coreClient        *dlcore.Client
}

var _ core.Downloader = &NativeHTTPDownloader{}

func NewNativeHTTPDownloader(cfg config.Downloader) *NativeHTTPDownloader {
	rootDir := cfg.Filesystem.RootDir
	logDirRel := cfg.Filesystem.LogDir
	logDir := ""
	if logDirRel != "" {
		logDir = filepath.Join(rootDir, logDirRel)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			slog.Error("Failed to create log directory", "dir", logDir, "error", err)
			logDir = ""
		}
	}

	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(cfg.Filesystem.RootDir, cfg.Filesystem.CacheDir)

	// 计算可选项需要的绝对路径
	var (
		moveEnabled bool
		moveDirAbs  string
		hlsLogEn    bool
		hlsLogPath  string
	)
	if cfg.FFmpeg.MoveIfExists.Enabled && cfg.FFmpeg.MoveIfExists.Dir != "" {
		moveEnabled = true
		moveDirAbs = filepath.Join(rootDir, cfg.FFmpeg.MoveIfExists.Dir)
	}
	if cfg.FFmpeg.ExternalHLSLog.Enabled && cfg.FFmpeg.ExternalHLSLog.Path != "" {
		hlsLogEn = true
		hlsLogPath = filepath.Join(rootDir, cfg.FFmpeg.ExternalHLSLog.Path)
	}

	// 创建配置化的 HTTP 客户端 [1,2](@ref)
	client := &http.Client{
		Timeout: time.Duration(cfg.HTTP.TimeoutSeconds) * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        cfg.HTTP.MaxIdleConns,
			MaxIdleConnsPerHost: cfg.HTTP.MaxIdleConnsPerHost,
			IdleConnTimeout:     time.Duration(cfg.HTTP.IdleConnTimeoutSeconds) * time.Second,
		},
	}

	coreClient := dlcore.NewClient(
		dlcore.WithHTTPClient(client),
		dlcore.WithRootDir(rootDir),
		dlcore.WithLoggerDir(logDir),
		dlcore.WithCacheDir(cacheDir),
		dlcore.WithDefaultUserAgent(cfg.HTTP.DefaultUserAgent),
		dlcore.WithDisableInjectBrowserLikeHeaders(cfg.HTTP.DisableInjectBrowserLikeHeaders),
		dlcore.WithProxies(cfg.Proxies),
		dlcore.WithForceProxy(cfg.ForceProxy),
		dlcore.WithProxyTuning(
			cfg.Proxy.DecisionCacheTTLSecs,
			cfg.Proxy.DirectProbeTimeoutSecs,
			cfg.Proxy.BandwidthPathSuffix,
		),
		dlcore.WithProgressTuning(cfg.Progress.MinPercentStep, cfg.Progress.MaxIntervalSeconds),
		dlcore.WithMaxRetries(cfg.MaxRetries),
		dlcore.WithFFmpegPath(cfg.FFmpeg.Path),
		dlcore.WithFFmpegExtraArgs(cfg.FFmpeg.ExtraArgs),
		dlcore.WithMoveIfExists(moveEnabled, moveDirAbs),
		dlcore.WithExternalHLSLog(hlsLogEn, hlsLogPath),
		dlcore.WithHLSAutoMarkAsFail(cfg.FFmpeg.HLSAutoMarkAsFail || cfg.HlsAutoMarkAsFail),
	)

	return &NativeHTTPDownloader{
		logDir:            logDir,
		proxies:           cfg.Proxies,
		cacheFile:         filepath.Join(home, ".config/download-manager/proxy_cache"),
		forceProxy:        cfg.ForceProxy,
		maxRetries:        cfg.MaxRetries,
		client:            client,
		ffmpegPath:        cfg.FfmpegPath,
		hlsAutoMarkAsFail: cfg.HlsAutoMarkAsFail,
		coreClient:        coreClient,
	}
}

func (d *NativeHTTPDownloader) SetContext(ctx context.Context) {
	d.ctx = ctx
}

func (d *NativeHTTPDownloader) ApplyDomainLimits(limits map[string]int) {
	if d.coreClient != nil {
		d.coreClient.ApplyDomainLimits(limits)
	}
}

func (d *NativeHTTPDownloader) Name() string {
	return "native_http"
}

func (d *NativeHTTPDownloader) Download(obj *model.DownloadObject, headers map[string]string) error {
	// 复合下载逻辑
	if filesVal, ok := obj.Extra["files"]; ok && filesVal != nil {
		var fileList []map[string]string

		if fmt.Sprintf("%T", filesVal) == "primitive.A" {
			filesVal = filesVal.([]any)
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
			return fmt.Errorf("%w: composite download error: 'files' metadata found but empty", dlcore.ErrNoTry)
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
					obj.SetProgress(int(p))
				},
				Metadata: fileMap,
			}
			dlCtx := d.ctx
			if dlCtx == nil {
				dlCtx = context.Background()
			}
			if err := d.coreClient.Download(dlCtx, req); err != nil {
				return err
			}
		}
		obj.SetProgress(100)
		return nil
	}

	// 单文件下载
	req := &dlcore.Request{
		URL:           obj.URL,
		SavePath:      obj.SavePath,
		Headers:       headers,
		TrackProgress: true,
		OnProgress: func(p float64, downloaded, total int64) {
			obj.SetProgress(int(p))
		},
		Metadata: obj.Metadata,
	}
	dlCtx := d.ctx
	if dlCtx == nil {
		dlCtx = context.Background()
	}
	if err := d.coreClient.Download(dlCtx, req); err != nil {
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
