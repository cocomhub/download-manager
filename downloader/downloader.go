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

// New 鍒涘缓 core.Downloader 瀹炰緥銆?// 鏍规嵁 config.Type 閫夋嫨鍚庣锛?//   - "wget": 浣跨敤鏃х殑 WgetDownloader锛堝凡搴熷純锛?//   - "native_old": 浣跨敤鏃х殑 NativeHTTPDownloader锛堜娇鐢ㄥ凡搴熷純鐨?pkg/dlcore锛?//   - "native" 鎴栭粯璁? 浣跨敤鏂扮殑 pkg/download.Downloader锛堥€氳繃閫傞厤鍣級
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

// newDownloaderFromConfig 浠庨厤缃瀯寤烘柊鐨?pkg/download 涓嬭浇鍣ㄣ€?func newDownloaderFromConfig(cfg config.Downloader) *DownloaderAdapter {
	// 鍒涘缓 StdlibTransport锛堝甫閰嶇疆鐨勮秴鏃跺拰杩炴帴姹犲弬鏁帮級
	tr := download.NewStdlibTransport()
	if len(cfg.DomainLimits) > 0 {
		tr.SetDomainLimits(cfg.DomainLimits)
	}

	// 鍒涘缓浠ｇ悊閫夋嫨鍣?	var sel download.Selector
	if len(cfg.Proxies) > 0 {
		ps := download.NewStaticProxySelector(cfg.Proxies)
		sel = download.NewDefaultSelector().WithProxySelector(ps)
	}

	// 鍒涘缓 Extractor 瀹炰緥锛堜紶閫掗厤缃弬鏁帮級
	userAgent := cfg.HTTP.DefaultUserAgent
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
	}

	httpEx := download.NewHTTPExtractorWithConfig(cfg.MaxRetries, userAgent, cfg.Filesystem.RootDir, cfg.Filesystem.LogDir)
	if len(cfg.Filesystem.AllowPaths) > 0 {
		httpEx.SetAllowPaths(cfg.Filesystem.AllowPaths)
	}
	hlsEx := extractor.NewHLSExtractor(
		extractor.WithFFmpegPath(cfg.FFmpeg.Path),
		extractor.WithFFmpegArgs(cfg.FFmpeg.ExtraArgs),
		extractor.WithHLSUserAgent(userAgent),
	)

	// 鍒涘缓涓嬭浇鍣?	reg := download.NewMetricRegistry()
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
	// 璁句负鍏ㄥ眬榛樿锛屼緵 manager/small_object.go 涓殑 download.Get() 浣跨敤
	download.SetDefault(dl)
	adapter := NewDownloaderAdapter(dl)

	// 娉ㄥ叆浼犺緭灞傚紩鐢ㄥ拰 metrics
	adapter.transport = tr
	adapter.metrics = reg

	return adapter
}
