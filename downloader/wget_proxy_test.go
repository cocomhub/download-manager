// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"strings"
	"testing"
)

// TestBuildWgetArgs_StandardProxy 验证旧 wget 后端标准代理（无前缀）：
// -e use_proxy=yes -e http_proxy=<proxy> + 原 URL。
func TestBuildWgetArgs_StandardProxy(t *testing.T) {
	t.Parallel()
	args := buildWgetArgs("https://njavtv.com/watch/123", "out.mp4", "http://127.0.0.1:1080", nil)
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

// TestBuildWgetArgs_GatewayProxy 验证旧 wget 后端网关式（gateway: 前缀）：拼 URL。
func TestBuildWgetArgs_GatewayProxy(t *testing.T) {
	t.Parallel()
	args := buildWgetArgs("https://njavtv.com/watch/123", "out.mp4", "gateway:http://127.0.0.1:1080", nil)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "http://127.0.0.1:1080/njavtv.com/watch/123") {
		t.Errorf("网关式缺拼 URL: %s", joined)
	}
}
