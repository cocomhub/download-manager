// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
)

// sseTestSetup 创建带指定鉴权配置的 Server，并启动真实 HTTP 测试服务器。
// 返回 server.URL（用 http://<host> 形式使 Origin 同源判断可比较）。
func sseTestSetup(t *testing.T, authType, token string) *httptest.Server {
	t.Helper()
	cfg := &config.Config{
		Server: config.Server{
			WorkDir: t.TempDir(),
			Auth: config.AuthConfig{
				Type:  authType,
				Token: token,
			},
		},
		Runtime: config.Runtime{
			Mode: config.RunModeFull,
			Download: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{Enabled: true},
			Scheduler: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{Enabled: true},
		},
	}
	srv := NewServer(newTestManager(cfg))
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)
	return ts
}

// sseGet 发起 SSE GET 请求并返回响应头（不等 body 结束 —— SSE 是长连接，
// 读头后立即关闭，验证 200 + event-stream 即可）。
func sseGet(t *testing.T, url, origin, authHeader string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	// 短超时：SSE 长连接读头后由 defer Close 断开。
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func TestSSE_SameOriginOK(t *testing.T) {
	ts := sseTestSetup(t, "none", "")

	resp := sseGet(t, ts.URL+"/api/events", ts.URL, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
}

func TestSSE_CrossOriginRejected(t *testing.T) {
	ts := sseTestSetup(t, "none", "")

	resp := sseGet(t, ts.URL+"/api/events", "http://evil.example", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
}

func TestSSE_TokenRequired(t *testing.T) {
	ts := sseTestSetup(t, "token", "mytoken")

	resp := sseGet(t, ts.URL+"/api/events", "", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestSSE_TokenAccepted(t *testing.T) {
	ts := sseTestSetup(t, "token", "mytoken")

	resp := sseGet(t, ts.URL+"/api/events", ts.URL, "Bearer mytoken")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
}

func TestSSE_NoOriginPasses(t *testing.T) {
	ts := sseTestSetup(t, "none", "")

	// curl 风格无 Origin 请求保持放行（API 兼容）。
	resp := sseGet(t, ts.URL+"/api/events", "", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestSSE_ContextCancelUnblocks(t *testing.T) {
	ts := sseTestSetup(t, "none", "")

	ctx, cancel := context.WithCancel(t.Context())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	cancel() // 取消 ctx → handler 的 r.Context().Done() 触发 → 返回
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}
