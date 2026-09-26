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

// DefaultUserAgent 默认浏览器 UA（原定义于已退役的 pkg/dlcore 时代的 native.go）。
var DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"

// New 创建 core.Downloader 实例。
// 根据 config.Type 选择后端：
//   - "wget": 使用旧的 WgetDownloader（已废弃）
//   - 其他或默认: 使用 pkg/download.Downloader（通过适配器）
//
// pkg/dlcore（native_old）已于 v0.1.0 后退役，旧配置自动回落到 native。
func New(cfg config.Downloader) core.Downloader {
	switch cfg.Type {
	case "wget":
		slog.Warn("wget backend is deprecated, use native instead")
		return NewWgetDownloader(cfg)
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
		// 接线 dlcore 已有的代理决策增强：直连探测超时/缓存 TTL/带宽后缀/强制代理。
		// （pkg/download 已实现这些能力，此前 downloader.New 未接线）
		ps.WithProbe(cfg.Proxy.DirectProbeTimeoutSecs)
		ps.WithCache(cfg.Filesystem.CacheDir, cfg.Proxy.DecisionCacheTTLSecs)
		ps.WithBandwidthSuffix(cfg.Proxy.BandwidthPathSuffix)
		if cfg.ForceProxy {
			ps.WithForceProxy(true)
		}
		sel = download.NewDefaultSelector().WithProxySelector(ps)
	}

	// 创建 Extractor 实例（传递配置参数）
	userAgent := cfg.HTTP.DefaultUserAgent
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
	}

	httpEx := download.NewHTTPExtractorWithConfig(cfg.MaxRetries, userAgent, cfg.Filesystem.RootDir, cfg.Filesystem.LogDir)
	if len(cfg.Filesystem.AllowPaths) > 0 {
		httpEx.SetAllowPaths(cfg.Filesystem.AllowPaths)
	}
	if cfg.Filesystem.FollowSymlinks != nil {
		httpEx.SetFollowSymlinks(*cfg.Filesystem.FollowSymlinks)
	}
	hlsEx := extractor.NewHLSExtractor(
		extractor.WithFFmpegPath(cfg.FFmpeg.Path),
		extractor.WithFFmpegArgs(cfg.FFmpeg.ExtraArgs),
		extractor.WithHLSUserAgent(userAgent),
	)
	// HLS 下载模式：ffmpeg（默认，需系统 ffmpeg）/ m3u8d（纯 Go 分片下载+拼接，无需 ffmpeg）
	if cfg.HLSMode != "" {
		hlsEx = extractor.NewHLSExtractor(
			extractor.WithHLSMode(cfg.HLSMode),
			extractor.WithFFmpegPath(cfg.FFmpeg.Path),
			extractor.WithFFmpegArgs(cfg.FFmpeg.ExtraArgs),
			extractor.WithHLSUserAgent(userAgent),
		)
	}

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
