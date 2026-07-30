// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"sync"
)

// Selector 是顶层选择器，同时负责匹配提取器和选择代理。
type Selector interface {
	// MatchExtractor 根据 URL 和提示信息返回匹配的 Extractor。
	MatchExtractor(ctx context.Context, url string, hint *DownloadHint) Extractor

	// SelectProxy 根据目标 URL 和提示信息返回代理 URL。
	SelectProxy(ctx context.Context, targetURL string, hint *DownloadHint) (proxyURL string, err error)
}

// ProxySelector 是仅负责代理选择的接口。
type ProxySelector interface {
	// Select 根据目标 URL 和提示信息返回代理 URL。
	Select(ctx context.Context, targetURL string, hint *DownloadHint) (proxyURL string, err error)
}

// DefaultSelector 是默认的 Selector 实现。
// 不再持有 extractors 列表，由 Downloader.matchExtractor 的 fallback 循环匹配。
// MatchExtractor 始终返回 nil，让调用方回退到自身的 extractors 列表。
// NewDefaultSelector 创建 DefaultSelector 实例。
func NewDefaultSelector() *DefaultSelector {
	return &DefaultSelector{}
}

type DefaultSelector struct {
	mu            sync.Mutex
	proxySelector ProxySelector
}

// WithProxySelector 设置代理选择器。
func (s *DefaultSelector) WithProxySelector(ps ProxySelector) *DefaultSelector {
	s.mu.Lock()
	s.proxySelector = ps
	s.mu.Unlock()
	return s
}

func (s *DefaultSelector) MatchExtractor(ctx context.Context, url string, hint *DownloadHint) Extractor {
	// DefaultSelector 不再持有 extractors 列表，
	// 由 Downloader.matchExtractor 的 fallback 循环匹配。
	// 此处始终返回 nil，让调用方回退到自身的 extractors 列表。
	return nil
}

func (s *DefaultSelector) SelectProxy(ctx context.Context, targetURL string, hint *DownloadHint) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps := s.proxySelector
	if ps != nil {
		return ps.Select(ctx, targetURL, hint)
	}
	return "", nil
}
