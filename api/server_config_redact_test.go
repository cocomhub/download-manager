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

// TestDownloaderConfigView_RedactsProxyListKey 验证 proxy.list（ValidateAndClamp 从顶层
// proxies 复制而来）同样被脱敏——此前误用不存在的 dc_proxy 键导致该分支为死代码。
func TestDownloaderConfigView_RedactsProxyListKey(t *testing.T) {
	t.Parallel()
	dl := config.Downloader{
		Proxies: []string{"http://bob:pw-list@10.0.0.1:3128"},
	}
	dl.Proxy.List = []string{"http://bob:pw-list@10.0.0.1:3128"}
	view := downloaderConfigView(dl)
	b, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "pw-list") {
		t.Fatalf("proxy.list 凭据泄漏: %s", b)
	}
}

// TestRedactYAMLSecrets 验证 diff 端点的 YAML 掩码覆盖机密键与代理 userinfo。
func TestRedactYAMLSecrets(t *testing.T) {
	t.Parallel()
	in := "downloader:\n  access_key_secret: sup3r-s3cret\n  api_token: bearer-xyz\n  proxies:\n    - http://u:p@1.2.3.4:8080\n"
	out := redactYAMLSecrets(in)
	for _, leak := range []string{"sup3r-s3cret", "bearer-xyz", "u:p@"} {
		if strings.Contains(out, leak) {
			t.Fatalf("YAML 未掩码 %q: %s", leak, out)
		}
	}
	if !strings.Contains(out, "1.2.3.4:8080") {
		t.Fatalf("主机应保留: %s", out)
	}
}

// TestSameRedactedProxies 验证「原样回传脱敏视图」被识别为未修改（保留已存凭据）。
func TestSameRedactedProxies(t *testing.T) {
	t.Parallel()
	current := []string{"http://u:p@1.2.3.4:8080"}
	if !sameRedactedProxies([]string{"http://1.2.3.4:8080"}, current) {
		t.Fatal("回传脱敏视图应判定为未修改")
	}
	if sameRedactedProxies([]string{"http://other:9@1.2.3.4:8080"}, current) {
		t.Fatal("显式修改应判定为已修改")
	}
	if sameRedactedProxies(nil, current) {
		t.Fatal("长度不同应判定为已修改")
	}
}
