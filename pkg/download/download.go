// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// Downloader 是用户使用的主要入口。
// 持有 Selector、Extractor 注册表、Transport 引用、Middleware 链和 Metrics，编排一次完整下载。
type Downloader struct {
	selector   Selector
	extractors []Extractor
	transport  Transport
	middleware Middleware
	metrics    *MetricRegistry
}

// ErrNoDefaultDownloader 表示未配置默认 Downloader。
var ErrNoDefaultDownloader = errors.New("default downloader not initialized; call download.SetDefault() or configure via download.New()")

// defaultDl 是包级默认 Downloader 实例，通过 SetDefault 配置。
var (
	defaultDl   *Downloader
	defaultDlMu sync.RWMutex
)

// SetDefault 替换包级默认 Downloader 实例。
// 调用后 Default() 和 Get() 将使用此实例。
func SetDefault(d *Downloader) {
	defaultDlMu.Lock()
	defaultDl = d
	defaultDlMu.Unlock()
}

// Default 返回包级默认 Downloader。首次调用时若未初始化，自动创建。
func Default() *Downloader {
	defaultDlMu.RLock()
	if defaultDl != nil {
		defaultDlMu.RUnlock()
		return defaultDl
	}
	defaultDlMu.RUnlock()

	defaultDlMu.Lock()
	defer defaultDlMu.Unlock()
	if defaultDl == nil {
		defaultDl = New()
	}
	return defaultDl
}

// Get 使用默认 Downloader 执行一次简单下载。
// 若默认实例未初始化，自动创建。
func Get(ctx context.Context, url, savePath string) error {
	return Default().Download(ctx, &Request{
		URL:      url,
		SavePath: savePath,
	})
}

// New 创建 Downloader，可通过 Option 自定义配置。
// 零参数调用时自动注册 HTTPExtractor、StdlibTransport、DefaultSelector。
// 若传入了 WithExtractor，则不注册默认 HTTPExtractor。
func New(opts ...Option) *Downloader {
	d := &Downloader{
		transport:  NewStdlibTransport(),
		selector:   NewDefaultSelector(),
		extractors: nil, // 由 initExtractors 惰性初始化
	}
	for _, o := range opts {
		o(d)
	}
	if d.extractors == nil {
		d.extractors = []Extractor{NewHTTPExtractor()}
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
	if req.Result == nil {
		req.Result = &DownloadResult{}
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

	// 3. 中间件链包装 Extractor
	executor := ex
	if d.middleware != nil {
		executor = &middlewareExtractor{
			base: ex,
			mw:   d.middleware,
		}
	}

	// 4. 执行下载
	return executor.Extract(ctx, req)
}
