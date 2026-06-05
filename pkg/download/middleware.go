// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"time"
)

// Middleware 是下载中间件函数类型。
// 在 Extract 前后执行额外逻辑，通过 next 参数调用下一个中间件或实际的 Extractor。
type Middleware func(ctx context.Context, req *Request, next Extractor) error

// MetricsMiddleware 创建记录下载指标的中间件。
func MetricsMiddleware(registry *MetricRegistry) Middleware {
	return func(ctx context.Context, req *Request, next Extractor) error {
		start := time.Now()
		err := next.Extract(ctx, req)
		elapsed := time.Since(start)

		registry.Record(next.Name(), 0, elapsed, err == nil)
		return err
	}
}

// middlewareExtractor 包装 Extractor，在 Extract 前后执行中间件逻辑。
type middlewareExtractor struct {
	base Extractor
	mw   Middleware
}

func (m *middlewareExtractor) Name() string { return m.base.Name() }

func (m *middlewareExtractor) Match(ctx context.Context, url string) bool {
	return m.base.Match(ctx, url)
}

func (m *middlewareExtractor) Extract(ctx context.Context, req *Request) error {
	return m.mw(ctx, req, m.base)
}
