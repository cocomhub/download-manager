// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "context"

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

// WithMiddleware 设置下载中间件。
// 如果之前已设置中间件，新中间件会包装在外部，形成链式调用。
// 注册顺序：[mw1, mw2] => 执行顺序: mw2_before -> mw1_before -> extractor -> mw1_after -> mw2_after
func WithMiddleware(mw Middleware) Option {
	return func(d *Downloader) {
		if d.middleware == nil {
			d.middleware = mw
			return
		}
		// 新中间件在最外层：新中间件调用 next（旧的中间件链）
		existing := d.middleware
		d.middleware = func(ctx context.Context, req *Request, next Extractor) error {
			return mw(ctx, req, &middlewareExtractor{
				base: next,
				mw:   existing,
			})
		}
	}
}

// WithMetricRegistry 设置指标注册表（并自动启用 MetricsMiddleware）。
func WithMetricRegistry(reg *MetricRegistry) Option {
	return func(d *Downloader) {
		d.metrics = reg
		mw := MetricsMiddleware(reg)
		if d.middleware == nil {
			d.middleware = mw
			return
		}
		// 链式包装：指标中间件在最外层记录总时间
		existing := d.middleware
		d.middleware = func(ctx context.Context, req *Request, next Extractor) error {
			return mw(ctx, req, &middlewareExtractor{
				base: next,
				mw:   existing,
			})
		}
	}
}

// WithRuleSet 设置 URL 路由规则集。
// RuleSet 用于在选择 Extractor 之前注解 Request 的 Hint 字段。
func WithRuleSet(rs *RuleSet) Option {
	return func(d *Downloader) {
		// 包装 selector，在匹配前应用规则集注解
		if d.selector == nil {
			d.selector = &ruleSetSelector{rs: rs}
			return
		}
		// 已有 selector，包装之
		base := d.selector
		d.selector = &ruleSetSelector{rs: rs, next: base}
	}
}

// ruleSetSelector 包装一个 Selector，在 MatchExtractor 前先通过 RuleSet 注解 Hint。
type ruleSetSelector struct {
	rs   *RuleSet
	next Selector
}

func (r *ruleSetSelector) MatchExtractor(ctx context.Context, url string, hint *DownloadHint) Extractor {
	// 先通过规则集匹配，若匹配则注解 Hint
	if matched := r.rs.Match(url, hint); matched != nil {
		if hint == nil {
			hint = &DownloadHint{Extractor: matched.Extractor}
		} else if matched.Extractor != "" {
			hint.Extractor = matched.Extractor
		}
	}
	if r.next != nil {
		return r.next.MatchExtractor(ctx, url, hint)
	}
	return nil
}

func (r *ruleSetSelector) SelectProxy(ctx context.Context, targetURL string, hint *DownloadHint) (string, error) {
	if r.next != nil {
		return r.next.SelectProxy(ctx, targetURL, hint)
	}
	return "", nil
}
