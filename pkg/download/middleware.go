// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"time"
)

// Middleware 鏄笅杞戒腑闂翠欢鍑芥暟绫诲瀷銆?// 鍦?Extract 鍓嶅悗鎵ц棰濆閫昏緫锛岄€氳繃 next 鍙傛暟璋冪敤涓嬩竴涓腑闂翠欢鎴栧疄闄呯殑 Extractor銆?type Middleware func(ctx context.Context, req *Request, next Extractor) error

// MetricsMiddleware 鍒涘缓璁板綍涓嬭浇鎸囨爣鐨勪腑闂翠欢銆?func MetricsMiddleware(registry *MetricRegistry) Middleware {
	return func(ctx context.Context, req *Request, next Extractor) error {
		start := time.Now()
		err := next.Extract(ctx, req)
		elapsed := time.Since(start)

		registry.Record(next.Name(), 0, elapsed, err == nil)
		return err
	}
}

// middlewareExtractor 鍖呰 Extractor锛屽湪 Extract 鍓嶅悗鎵ц涓棿浠堕€昏緫銆?type middlewareExtractor struct {
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
