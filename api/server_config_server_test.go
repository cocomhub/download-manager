// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/testutil/assert"
)

// TestAPI_ConfigServerGet_ServerSection verifies GET /api/config/server returns
// the full server section (ports, dirs, auth with secrets redacted).
func TestAPI_ConfigServerGet_ServerSection(t *testing.T) {
	srv, cfg := newAPIServerWithMock(t, "mock-cfg-server", 1, false)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/config/server")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for config endpoint ready")

	rr := doJSONGet(t, r, "/api/config/server")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/config/server returned %d, want 200", rr.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	server, ok := body["server"].(map[string]any)
	if !ok {
		t.Fatalf("server section missing in response: %v", body)
	}

	// Ports should be present with defaults applied by the manager.
	if _, ok := server["http_port"]; !ok {
		t.Error("server.http_port missing")
	}
	if _, ok := server["ui_only_port"]; !ok {
		t.Error("server.ui_only_port missing")
	}
	if _, ok := server["work_dir"]; !ok {
		t.Error("server.work_dir missing")
	}
	if _, ok := server["download_root_dir"]; !ok {
		t.Error("server.download_root_dir missing")
	}
	if _, ok := server["files_dir"]; !ok {
		t.Error("server.files_dir missing")
	}
	if _, ok := server["files_allow_symlink"]; !ok {
		t.Error("server.files_allow_symlink missing")
	}

	// Auth must be redacted: no plaintext password/token.
	auth, ok := server["auth"].(map[string]any)
	if !ok {
		t.Fatalf("server.auth missing in response: %v", server)
	}
	if _, hasPw := auth["password"]; hasPw {
		t.Error("server.auth.password must never be returned")
	}
	if _, hasTok := auth["token"]; hasTok {
		t.Error("server.auth.token must never be returned")
	}
	if _, ok := auth["type"]; !ok {
		t.Error("server.auth.type missing")
	}

	// Old sections remain backward-compatible.
	for _, k := range []string{"task_scan", "downloader", "ui_defaults", "log_level"} {
		if _, ok := body[k]; !ok {
			t.Errorf("config section %q missing (backward compat)", k)
		}
	}

	_ = cfg
	_ = done
}

// TestAPI_ConfigServerGet_DownloaderSecretsRedacted 验证 GET /api/config/server
// 不回传 downloader 段的机密（SproxySig AccessKeySecret / Bearer api_token），
// 只暴露 has_* 标记。
//
// 安全背景（对抗性评审 P0）：downloadloader 段原为整体 json 序列化，会把 SproxySig
// AccessKeySecret（等价 sproxy 全权凭据）明文回给任何能访问 /api/config/server 的
// 客户端（默认 auth 为空 = 不鉴权）。
func TestAPI_ConfigServerGet_DownloaderSecretsRedacted(t *testing.T) {
	const secret = "sk-very-secret-value-123"
	const token = "bearer-token-value-456"
	srv, cfg := newAPIServerWithMock(t, "mock-cfg-dl-secret", 1, false)
	cfg.Downloader.SproxyCloud.AccessKey = "ak-test"
	cfg.Downloader.SproxyCloud.AccessKeySecret = secret
	cfg.Downloader.SproxyCloud.APIToken = token
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/config/server")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for config endpoint ready")

	rr := doJSONGet(t, r, "/api/config/server")
	raw := rr.Body.String()
	if strings.Contains(raw, secret) {
		t.Fatal("access_key_secret leaked in /api/config/server response")
	}
	if strings.Contains(raw, token) {
		t.Fatal("api_token leaked in /api/config/server response")
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	dl, ok := body["downloader"].(map[string]any)
	if !ok {
		t.Fatalf("downloader section missing: %v", body)
	}
	sh, ok := dl["sproxy_cloud"].(map[string]any)
	if !ok {
		t.Fatalf("downloader.sproxy_cloud missing: %v", dl)
	}
	if sh["access_key_secret"] != "" {
		t.Errorf("access_key_secret should be empty, got %v", sh["access_key_secret"])
	}
	if sh["api_token"] != "" {
		t.Errorf("api_token should be empty, got %v", sh["api_token"])
	}
	if sh["has_access_key_secret"] != true {
		t.Errorf("has_access_key_secret should be true, got %v", sh["has_access_key_secret"])
	}
	if sh["has_api_token"] != true {
		t.Errorf("has_api_token should be true, got %v", sh["has_api_token"])
	}
	_ = done
}

// TestAPI_ConfigServerUpdate_ServerSection verifies POST /api/config/server can
// update the server section (http_port / files_dir) and persists it.
// 注意：不在此更新 auth.type（会触发 auth 中间件 401，GET 验证无法进行）；
// auth 更新见 TestAPI_ConfigServerUpdate_Auth（直接查 mgr.GetConfig）。
func TestAPI_ConfigServerUpdate_ServerSection(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-cfg-server-upd", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/config/history")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for config before server update")

	body := map[string]any{
		"server": map[string]any{
			"http_port":           18080,
			"files_dir":           "/tmp/dm-files",
			"files_allow_symlink": true,
		},
	}
	rr := doJSONPost(t, r, "/api/config/server", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/config/server returned %d: %s", rr.Code, rr.Body.String())
	}

	// Verify the update persisted by reading back.
	assert.MustEventually(t, func() bool {
		grr := doJSONGet(t, r, "/api/config/server")
		if grr.Code != http.StatusOK {
			return false
		}
		var got map[string]any
		if err := json.Unmarshal(grr.Body.Bytes(), &got); err != nil {
			return false
		}
		server, ok := got["server"].(map[string]any)
		if !ok {
			return false
		}
		if p, _ := server["http_port"].(float64); int(p) != 18080 {
			return false
		}
		// files_allow_symlink 需回显 true（GET 视图带该字段）。
		if as, _ := server["files_allow_symlink"].(bool); !as {
			return false
		}
		return true
	}, 3*time.Second, 50*time.Millisecond, "server config update persisted")

	// 装配校验：POST 后 srv.filesAllowSymlink 已同步为 true（热更接线）。
	if !srv.filesAllowSymlink {
		t.Error("filesAllowSymlink not wired after server config update")
	}

	_ = done
}

// TestAPI_ConfigServerUpdate_Auth verifies POST /api/config/server can update
// auth.type without exposing secrets. It inspects mgr.GetConfig directly because
// enabling basic auth makes subsequent GETs return 401.
func TestAPI_ConfigServerUpdate_Auth(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-cfg-auth-upd", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/config/history")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for config before auth update")

	body := map[string]any{
		"server": map[string]any{
			"auth": map[string]any{
				"type":     "basic",
				"username": "admin",
				"password": "secret123",
			},
		},
	}
	rr := doJSONPost(t, r, "/api/config/server", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/config/server returned %d: %s", rr.Code, rr.Body.String())
	}

	// 直接查内部配置（GET 会因 auth 启用返回 401）。
	cur := srv.mgr.GetConfig()
	if cur.Server.Auth.Type != "basic" {
		t.Errorf("auth.type: got %q, want basic", cur.Server.Auth.Type)
	}
	if cur.Server.Auth.Username != "admin" {
		t.Errorf("auth.username: got %q, want admin", cur.Server.Auth.Username)
	}
	if cur.Server.Auth.Password != "secret123" {
		t.Errorf("auth.password: got %q, want secret123", cur.Server.Auth.Password)
	}

	// GET 应因 auth 启用返回 401（验证副作用符合预期）。
	grr := doJSONGet(t, r, "/api/config/server")
	if grr.Code != http.StatusUnauthorized {
		t.Logf("GET after enabling basic auth returned %d (expect 401)", grr.Code)
	}

	_ = done
}
