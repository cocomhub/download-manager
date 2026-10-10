// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cocomhub/download-manager/config"
)

// TestDownloaderConfigView_RedactsProxyCredentials 验证代理列表内联的 user:pass 不回泄。
func TestDownloaderConfigView_RedactsProxyCredentials(t *testing.T) {
	t.Parallel()
	dl := config.Downloader{
		Proxies: []string{"http://alice:s3cret-pw@127.0.0.1:8080", "socks5://127.0.0.1:1080"},
	}
	view := downloaderConfigView(dl)
	b, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "s3cret-pw") {
		t.Fatalf("代理凭据泄漏: %s", b)
	}
	if !strings.Contains(string(b), "127.0.0.1:8080") {
		t.Fatalf("代理主机应保留: %s", b)
	}
}
