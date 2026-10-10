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
	for _, leak := range []string{"sup3r-s3cret", "bearer-xyz", "tok-abc", "u:p@"} {
		if strings.Contains(out, leak) {
			t.Fatalf("YAML 未掩码 %q: %s", leak, out)
		}
	}
	if !strings.Contains(out, "1.2.3.4:8080") {
		t.Fatalf("主机应保留: %s", out)
	}
}

// TestMergeRedactedProxies 验证脱敏视图回写时逐条保留已存凭据（含混合编辑场景）。
func TestMergeRedactedProxies(t *testing.T) {
	t.Parallel()
	current := []string{"http://u:p@1.2.3.4:8080", "socks5://1.1.1.1:1080"}

	// 原样回传脱敏视图 → 全部保留凭据
	got := mergeRedactedProxies([]string{"http://1.2.3.4:8080", "socks5://1.1.1.1:1080"}, current)
	if got[0] != current[0] || got[1] != current[1] {
		t.Fatalf("原样回传应保留凭据, got %v", got)
	}
	// 混合编辑：保留原代理 + 新增一条 → 原代理凭据不丢
	got = mergeRedactedProxies([]string{"http://1.2.3.4:8080", "http://new:9@9.9.9.9:3128"}, current)
	if got[0] != current[0] {
		t.Fatalf("已存代理凭据应保留, got %v", got)
	}
	if got[1] != "http://new:9@9.9.9.9:3128" {
		t.Fatalf("新增项应原样采用, got %v", got)
	}
	// 空请求 → 不动当前配置
	if got := mergeRedactedProxies(nil, current); len(got) != len(current) {
		t.Fatalf("空请求不应清空, got %v", got)
	}
}

// TestRedactDiffChanges 验证 diff 结构化 changes 中的代理列表也被脱敏（此前是旁路）。
func TestRedactDiffChanges(t *testing.T) {
	t.Parallel()
	res := map[string]any{
		"changes": []config.Change{
			{Path: "downloader.proxies", A: []string{"http://u:p@1.2.3.4:8080"}, B: []string{"http://v:q@5.6.7.8:8080"}},
			{Path: "downloader.type", A: "native", B: "wget"},
		},
	}
	redactDiffChanges(res)
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, leak := range []string{"u:p@", "v:q@"} {
		if strings.Contains(string(b), leak) {
			t.Fatalf("changes 未脱敏 %q: %s", leak, b)
		}
	}
	if !strings.Contains(string(b), "1.2.3.4:8080") {
		t.Fatalf("主机应保留: %s", b)
	}
}
