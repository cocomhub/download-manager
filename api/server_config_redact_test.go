// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
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
	// nil（字段未提供）→ 不动当前配置
	if got := mergeRedactedProxies(nil, current); len(got) != len(current) {
		t.Fatalf("nil 不应清空, got %v", got)
	}
	// 显式空切片 → 清空
	if got := mergeRedactedProxies([]string{}, current); len(got) != 0 {
		t.Fatalf("显式空列表应清空, got %v", got)
	}
	// 同脱敏视图重复项 → 不回填（避免凭据错配）
	dup := []string{"http://u1:p1@h:8080", "http://u2:p2@h:8080"}
	gotDup := mergeRedactedProxies([]string{"http://h:8080", "http://h:8080"}, dup)
	if gotDup[0] != "http://h:8080" || gotDup[1] != "http://h:8080" {
		t.Fatalf("重复脱敏视图不应回填凭据, got %v", gotDup)
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

// TestRedactYAMLSecrets_QuotedAndFlow 验证引号键与流式映射中的机密也被掩码，
// 且不误伤 has_* 前缀键。
func TestRedactYAMLSecrets_QuotedAndFlow(t *testing.T) {
	t.Parallel()
	in := "downloader:\n  \"api_token\": \"bearer-q\"\n  proxy: {token: flow-tok}\n  sproxy_cloud:\n    has_api_token: true\n"
	out := redactYAMLSecrets(in)
	for _, leak := range []string{"bearer-q", "flow-tok"} {
		if strings.Contains(out, leak) {
			t.Fatalf("未掩码 %q: %s", leak, out)
		}
	}
	if !strings.Contains(out, "has_api_token: true") {
		t.Fatalf("has_* 前缀键不应被误伤: %s", out)
	}
}

// TestAPI_DiffConfig_RejectsTraversal 验证 /api/config/diff 拒绝逃出 config_backups 的 ref。
func TestAPI_DiffConfig_RejectsTraversal(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-diff", 1, true)
	r := srv.Router()
	rr := doJSONGet(t, r, "/api/config/diff?left=../../../etc/passwd&right=current")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("目录穿越 ref 应 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	// 必须是被路径校验拒绝（而非「文件不存在」等其它原因）
	if !strings.Contains(rr.Body.String(), "invalid backup ref") {
		t.Fatalf("应以 invalid backup ref 拒绝, got %s", rr.Body.String())
	}
}

// TestAPI_ConfigBackupEndpoints_RejectTraversal 验证备份类端点统一拒绝目录穿越
// （此前只有 diff 校验，delete/rollback/tag/note 仍可越界读写删）。
func TestAPI_ConfigBackupEndpoints_RejectTraversal(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-backup-ref", 1, true)
	r := srv.Router()
	const evil = "../../evil"
	for _, ep := range []struct {
		url  string
		body map[string]any
	}{
		{"/api/config/delete", map[string]any{"filename": evil}},
		{"/api/config/rollback", map[string]any{"filename": evil}},
		{"/api/config/tag", map[string]any{"filename": evil, "tag": "t"}},
		{"/api/config/note", map[string]any{"filename": evil, "message": "m"}},
	} {
		rr := doJSONPost(t, r, ep.url, ep.body)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s 应拒绝穿越 ref, got %d body=%s", ep.url, rr.Code, rr.Body.String())
			continue
		}
		if !strings.Contains(rr.Body.String(), "invalid backup ref") {
			t.Errorf("%s 应以 invalid backup ref 拒绝, got %s", ep.url, rr.Body.String())
		}
	}
}

// TestRedactDiffChanges_MasksContextsAndExtra 验证结构化 changes 的递归掩码覆盖
// contexts（mongo uri 内联凭据）与 tasks extra（headers.Cookie）。
func TestRedactDiffChanges_MasksContextsAndExtra(t *testing.T) {
	t.Parallel()
	res := map[string]any{
		"changes": []config.Change{
			// 真实生产形状：map[string]config.Context（此前测试用 map[string]any → 假绿）
			{Path: "contexts", A: map[string]config.Context{
				"ctx1": {Storage: config.StorageConfig{
					Type:   "mongo",
					Config: map[string]string{"uri": "mongodb://root:root123@db:27017"},
				}},
			}},
			{Path: "tasks.t1.extra", A: map[string]any{
				"headers": map[string]any{"Cookie": "session=abc", "User-Agent": "ua"},
			}},
		},
	}
	redactDiffChanges(res)
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, leak := range []string{"root123", "session=abc"} {
		if strings.Contains(string(b), leak) {
			t.Fatalf("changes 未掩码 %q: %s", leak, b)
		}
	}
	if !strings.Contains(string(b), "db:27017") || !strings.Contains(string(b), "User-Agent") {
		t.Fatalf("非机密内容应保留: %s", b)
	}
}
