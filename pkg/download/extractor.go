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

// TransportSetter 是 Extractor 可选实现的接口，用于接收 Transport 实例。
type TransportSetter interface {
	SetTransport(Transport)
}

// SelectorSetter 是 Extractor 可选实现的接口，用于接收 Selector 实例。
type SelectorSetter interface {
	SetSelector(Selector)
}

// ResponseCheck 是 HTTP 响应校验函数。在 tryDownload 拿到响应后、写文件之前调用。
// 返回 error 则终止下载（ErrNoTry 表示永久终止，其他 error 可重试）。
type ResponseCheck func(req *Request, tresp *TransportResponse) error
