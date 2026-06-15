// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"log/slog"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/download/extractor"
)

// New 创建 core.Downloader 实例。
// 根据 config.Type 选择后端：
//   - "wget": 使用旧的 WgetDownloader（已废弃）
//   - "native_old": 使用旧的 NativeHTTPDownloader（使用已废弃的 pkg/dlcore）
//   - "native" 或默认: 使用新的 pkg/download.Downloader（通过适配器）
func New(cfg config.Downloader) core.Downloader {
	switch cfg.Type {
	case "wget":
		slog.Warn("wget backend is deprecated, use native instead")
		return NewWgetDownloader(cfg)
	case "native_old":
		slog.Warn("native_old uses deprecated pkg/dlcore, migrate to native (new pkg/download path)")
		return NewNativeHTTPDownloader(cfg)
	default:
		return newDownloaderFromConfig(cfg)
	}
}

// newDownloaderFromConfig 从配置构建新的 pkg/download 下载器。
func newDownloaderFromConfig(cfg config.Downloader) *DownloaderAdapter {
	// 创建 StdlibTransport（带配置的超时和连接池参数）
	tr := download.NewStdlibTransport()
	if len(cfg.DomainLimits) > 0 {
		tr.SetDomainLimits(cfg.DomainLimits)
	}

	// 创建代理选择器
	var sel download.Selector
	if len(cfg.Proxies) > 0 {
		ps := download.NewStaticProxySelector(cfg.Proxies)
		sel = download.NewDefaultSelector().WithProxySelector(ps)
	}

	// 创建 Extractor 实例（传递配置参数）
	userAgent := cfg.HTTP.DefaultUserAgent
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
	}

	httpEx := download.NewHTTPExtractorWithConfig(cfg.MaxRetries, userAgent, cfg.Filesystem.RootDir, cfg.Filesystem.LogDir)
	hlsEx := extractor.NewHLSExtractor(
		extractor.WithFFmpegPath(cfg.FFmpeg.Path),
		extractor.WithFFmpegArgs(cfg.FFmpeg.ExtraArgs),
		extractor.WithHLSUserAgent(userAgent),
	)

	// 创建下载器
	reg := download.NewMetricRegistry()
	opts := []download.Option{
		download.WithTransport(tr),
		download.WithExtractor(httpEx),
		download.WithExtractor(hlsEx),
		download.WithMetricRegistry(reg),
	}
	if sel != nil {
		opts = append(opts, download.WithSelector(sel))
	}

	dl := download.New(opts...)
	// 设为全局默认，供 manager/small_object.go 中的 download.Get() 使用
	download.SetDefault(dl)
	adapter := NewDownloaderAdapter(dl)

	// 注入传输层引用和 metrics
	adapter.transport = tr
	adapter.metrics = reg

	return adapter
}
