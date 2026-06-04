// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

// Option 是 Downloader 的配置函数。
type Option func(*Downloader)

// WithSelector 注入自定义 Selector。
func WithSelector(s Selector) Option {
	return func(d *Downloader) { d.selector = s }
}

// WithTransport 注入自定义 Transport。
func WithTransport(t Transport) Option {
	return func(d *Downloader) { d.transport = t }
}

// WithExtractor 注册 Extractor。
func WithExtractor(ex Extractor) Option {
	return func(d *Downloader) {
		d.extractors = append(d.extractors, ex)
	}
}