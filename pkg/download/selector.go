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

// ProxyFailureReporter 是可选接口：代理选择器可实现它，
// 以便在下载经某代理失败（连接错误 / 超时 / 非 2xx）时上报故障，
// 驱动代理池的轮换与故障切换（冷却后恢复）。
type ProxyFailureReporter interface {
	// ReportProxyFailure 标记指定代理的一次失败。
	ReportProxyFailure(proxyURL string)
}

// DownloadResultReporter 是可选接口：代理选择器可实现它，
// 以便下载成功后回写域名维度的真实下载结果（P7-6）。
// 直连成功 → ReportResult(domain, "direct")；代理成功 → ReportResult(domain, "proxy")。
// 缓存命中后直接优先对应通道（跳过探测/带宽扫描）。
type DownloadResultReporter interface {
	// ReportResult 记录某域名一次真实下载结果（channel ∈ direct/proxy）。
	ReportResult(domain, channel string)
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

// WithProxySelector 设置代理选择器并返回自身，支持链式调用。
// 注意：这是 DefaultSelector 中唯一返回自身的 setter，其他 setter 可能不遵循此模式。
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

// ReportResult 转发真实下载结果到代理选择器（若实现 DownloadResultReporter）。
// 无代理选择器（或未实现）时静默忽略——直连/代理决策缓存非强制。
func (s *DefaultSelector) ReportResult(domain, channel string) {
	s.mu.Lock()
	ps := s.proxySelector
	s.mu.Unlock()
	r, ok := ps.(DownloadResultReporter)
	if !ok {
		return
	}
	r.ReportResult(domain, channel)
}

// ReportProxyFailure 转发代理失败到代理选择器（若实现 ProxyFailureReporter）。
func (s *DefaultSelector) ReportProxyFailure(proxyURL string) {
	s.mu.Lock()
	ps := s.proxySelector
	s.mu.Unlock()
	r, ok := ps.(ProxyFailureReporter)
	if !ok {
		return
	}
	r.ReportProxyFailure(proxyURL)
}
