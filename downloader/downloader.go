// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"log/slog"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/download/extractor"
	"github.com/cocomhub/download-manager/pkg/download/transport"
)

// New 创建 core.Downloader 实例。
// 根据 config.Type 选择后端：
//   - "wget": 使用旧的 WgetDownloader（已废弃）
//   - "native" 或默认: 使用新的 pkg/download.Downloader（通过适配器）
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
	// 创建 StdlibTransport
	tr := transport.NewStdlibTransport()
	if len(cfg.DomainLimits) > 0 {
		tr.SetDomainLimits(cfg.DomainLimits)
	}

	// 创建代理选择器
	var sel download.Selector
	if len(cfg.Proxies) > 0 {
		ps := download.NewStaticProxySelector(cfg.Proxies)
		sel = download.NewDefaultSelector().WithProxySelector(ps)
	}

	// 注册 Extractor
	httpEx := extractor.NewHTTPExtractor()
	hlsEx := extractor.NewHLSExtractor()

	// 创建下载器
	opts := []download.Option{
		download.WithTransport(tr),
		download.WithExtractor(httpEx),
		download.WithExtractor(hlsEx),
	}
	if sel != nil {
		opts = append(opts, download.WithSelector(sel))
	}

	dl := download.New(opts...)
	adapter := NewDownloaderAdapter(dl)

	// 注入传输层引用（用于 ApplyDomainLimits）
	adapter.transport = tr

	return adapter
}