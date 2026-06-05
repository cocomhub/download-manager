// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "context"

// Extractor 接口负责根据 URL 和请求信息提取出最终的可下载资源。
// 不同的实现对应不同的提取策略（如原生直链、scraper、m3u8 解析等）。
type Extractor interface {
	// Name 返回提取器的名称。
	Name() string

	// Match 判断该提取器是否能够处理给定的 URL。
	Match(ctx context.Context, url string) bool

	// Extract 对请求进行提取处理，可能会修改 req 的字段（如 URL、Headers）。
	Extract(ctx context.Context, req *Request) error
}

// Canceller 表示支持取消正在进行的下载的 Extractor。
type Canceller interface {
	// Cancel 取消指定 URL 的下载。
	Cancel(url string) error
}
