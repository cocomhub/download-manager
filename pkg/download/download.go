// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// Downloader 是用户使用的主要入口。
// 持有 Selector、Extractor 注册表和 Transport 引用，编排一次完整下载。
type Downloader struct {
	selector   Selector
	extractors []Extractor
	transport  Transport
}

// ErrNoDefaultDownloader 表示未配置默认 Downloader。
var ErrNoDefaultDownloader = errors.New("default downloader not initialized; call download.SetDefault() or configure via download.New()")

// defaultDl 是包级默认 Downloader 实例，通过 SetDefault 配置。
var defaultDl *Downloader

// SetDefault 替换包级默认 Downloader 实例。
// 调用后 Default() 和 Get() 将使用此实例。
func SetDefault(d *Downloader) {
	defaultDl = d
}

// Default 返回包级默认 Downloader。若未初始化返回 nil。
func Default() *Downloader {
	return defaultDl
}

// Get 使用默认 Downloader 执行一次简单下载。
// 等效于 Default().Download(ctx, &Request{URL: url, SavePath: savePath})。
// 若未调用 SetDefault 初始化则返回 ErrNoDefaultDownloader。
func Get(ctx context.Context, url, savePath string) error {
	if defaultDl == nil {
		return ErrNoDefaultDownloader
	}
	return defaultDl.Download(ctx, &Request{
		URL:      url,
		SavePath: savePath,
	})
}

// New 创建 Downloader，可通过 Option 自定义配置。
func New(opts ...Option) *Downloader {
	d := &Downloader{
		extractors: make([]Extractor, 0),
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Download 执行一次下载的完整编排：
//  1. Selector 匹配 Extractor
//  2. 注入 Transport 和 Selector（如果 Extractor 支持）
//  3. 执行 Extractor.Extract
func (d *Downloader) Download(ctx context.Context, req *Request) error {
	if req == nil || req.URL == "" || req.SavePath == "" {
		return fmt.Errorf("invalid request: missing URL or SavePath")
	}
	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}

	// 1. Selector 匹配 Extractor
	var ex Extractor
	if d.selector != nil {
		ex = d.selector.MatchExtractor(ctx, req.URL, req.Hint)
	}
	if ex == nil {
		for _, e := range d.extractors {
			if e.Match(ctx, req.URL) {
				ex = e
				break
			}
		}
	}
	if ex == nil {
		return fmt.Errorf("no extractor found for URL: %s", req.URL)
	}

	slog.Debug("Download: matched extractor", "extractor", ex.Name(), "url", req.URL)

	// 2. 为 Extractor 注入 Transport 和 Selector（如果支持）
	if hw, ok := ex.(interface{ SetTransport(Transport) }); ok && d.transport != nil {
		hw.SetTransport(d.transport)
	}
	if hw, ok := ex.(interface{ SetSelector(Selector) }); ok && d.selector != nil {
		hw.SetSelector(d.selector)
	}

	// 3. 执行下载
	return ex.Extract(ctx, req)
}