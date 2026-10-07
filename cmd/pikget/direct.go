// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"

	"github.com/cocomhub/download-manager/pkg/download"
)

// httpOpts 是直链后端的运行参数。
type httpOpts struct {
	userAgent string
	proxyURL  string
	headers   map[string]string
	maxRetry  int
}

// downloadDirect 复用 pkg/download 下载普通 http(s) 直链。
// 继承能力：Range 断点续传、弱 ETag 条件请求、MD5 校验、失败重试（maxRetry）。
func downloadDirect(ctx context.Context, url, dest string, o httpOpts, onProg func(p float64, downloaded, total int64)) error {
	ex := download.NewHTTPExtractorWithConfig(o.maxRetry, o.userAgent, "", "")
	opts := []download.Option{download.WithExtractor(ex)}
	if o.proxyURL != "" {
		ps := download.NewStaticProxySelector([]string{o.proxyURL})
		opts = append(opts, download.WithSelector(download.NewDefaultSelector().WithProxySelector(ps)))
	}
	dl := download.New(opts...)
	req := &download.Request{
		URL:           url,
		SavePath:      dest,
		Headers:       o.headers,
		TrackProgress: onProg != nil,
		OnProgress:    onProg,
	}
	if req.Headers == nil {
		req.Headers = map[string]string{}
	}
	if o.userAgent != "" {
		if _, ok := req.Headers["User-Agent"]; !ok {
			req.Headers["User-Agent"] = o.userAgent
		}
	}
	if err := dl.Download(ctx, req); err != nil {
		return fmt.Errorf("pikget direct: %w", err)
	}
	return nil
}
