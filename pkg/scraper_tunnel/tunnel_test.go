// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tunnel

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	spxytunnel "github.com/cocomhub/sproxy/pkg/tunnel"
)

// testKey 返回 64 hex 测试密钥（AES-256）。
func testKey() string {
	return "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
}

// TestTunnelRequest_ViaSproxy 验证 TunnelRequest 经 sproxy 最新隧道协议转发：
// 目标服务返回的内容原样到达（加密隧道端到端）。
func TestTunnelRequest_ViaSproxy(t *testing.T) {
	t.Parallel()
	key := testKey()

	// 目标服务（隧道另一端真实 HTTP 服务）。
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("tunnel-target-ok"))
	}))
	defer target.Close()

	// sproxy 隧道服务端：NewLocalHandler 包目标（key 需 []byte；
	// 生产 /tunnel 由 authMiddleware 把 SK 派生密钥放入 ctx，测试手动注入）。
	kb, perr := spxytunnel.ParseKey(key)
	if perr != nil {
		t.Fatal(perr)
	}
	inner := spxytunnel.NewLocalHandler(kb, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}), slog.New(slog.NewTextHandler(&strings.Builder{}, nil)))
	withKey := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner.ServeHTTP(w, r.WithContext(spxytunnel.SetTunnelKey(r.Context(), kb)))
	})
	local := httptest.NewServer(withKey)
	defer local.Close()

	body, err := TunnelRequest(&SclientConfig{
		ServerURL:      local.URL,
		TunnelEndpoint: "", // NewLocalHandler 就是隧道端点本身
		Timeout:        10,
		TunnelKey:      key,
	}, "GET", target.URL, nil, "", false, false)
	if err != nil {
		t.Fatalf("TunnelRequest err: %v", err)
	}
	if !strings.Contains(body, "tunnel-target-ok") {
		t.Errorf("body = %q, want tunnel-target-ok", body)
	}
}
