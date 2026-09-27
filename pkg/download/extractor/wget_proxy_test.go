// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"strings"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

func newTestWgetExtractor() *WgetExtractor {
	// 直接构造（测 buildWgetArgs 纯函数，不要求本机装 wget）。
	return &WgetExtractor{
		userAgent:              "TestUA",
		maxRetries:             3,
		timeoutSecs:            30,
		redactSensitiveHeaders: true,
	}
}

// TestWgetBuildArgs_StandardProxy 验证无前缀代理 = wget 标准代理语义：
// -e use_proxy=yes -e http_proxy=<proxy> + 原 URL。
func TestWgetBuildArgs_StandardProxy(t *testing.T) {
	t.Parallel()
	e := newTestWgetExtractor()
	req := &download.Request{URL: "https://njavtv.com/watch/123", SavePath: "out.mp4"}
	args := e.buildWgetArgs(req, "http://127.0.0.1:1080")
	joined := strings.Join(args, " ")
	for _, want := range []string{"use_proxy=yes", "http_proxy=http://127.0.0.1:1080", "https://njavtv.com/watch/123"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args 缺 %q: %s", want, joined)
		}
	}
	if strings.Contains(joined, "1080/njavtv") {
		t.Errorf("标准代理不应拼网关 URL: %s", joined)
	}
}

// TestWgetBuildArgs_GatewayProxy 验证 gateway: 前缀 = 拼 URL 网关式。
func TestWgetBuildArgs_GatewayProxy(t *testing.T) {
	t.Parallel()
	e := newTestWgetExtractor()
	req := &download.Request{URL: "https://njavtv.com/watch/123", SavePath: "out.mp4"}
	args := e.buildWgetArgs(req, "gateway:http://127.0.0.1:1080")
	joined := strings.Join(args, " ")
	for _, want := range []string{"http://127.0.0.1:1080/njavtv.com/watch/123"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args 缺网关 URL %q: %s", want, joined)
		}
	}
}
