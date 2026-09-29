// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestSystemShutdown_Returns202 触发排空端点返回 202；StartDrain 幂等（只真正启动一次），
// drainTrigger 每次调用都会通知 main（channel 缓冲，幂等消费）。
func TestSystemShutdown_Returns202(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "drain-api", 1, true)
	r := srv.Router()

	var trig atomic.Int32
	srv.SetDrainTrigger(func() { trig.Add(1) })

	startAPIManager(t, srv)

	rr := doJSONPost(t, r, "/api/system/shutdown", map[string]any{})
	if rr.Code != http.StatusAccepted {
		t.Fatalf("POST /api/system/shutdown returned %d, want 202; body=%s", rr.Code, rr.Body.String())
	}
	if trig.Load() != 1 {
		t.Fatal("drain trigger callback should be invoked on first trigger")
	}
	if !srv.mgr.DrainRequested() {
		t.Fatal("manager should be in drain mode after trigger")
	}
	if !srv.mgr.DrainStartedOnce() {
		t.Fatal("StartDrain should report it started the drain")
	}

	// 幂等：再次触发仍 202。StartDrain 只真正启动一次（drainMode CAS），
	// 但每次调用仍会通知 main 的 drainTrigger（channel 幂等消费，无害）。
	rr2 := doJSONPost(t, r, "/api/system/shutdown", map[string]any{})
	if rr2.Code != http.StatusAccepted {
		t.Fatalf("second POST returned %d, want 202", rr2.Code)
	}
}

// doShutdownPost 向 /api/system/shutdown 发起 POST，可指定 Host 与 Origin 头
// （httptest.NewRequest 对相对 URL 不填 Host，需显式设置以模拟真实请求）。
func doShutdownPost(t *testing.T, router http.Handler, host, origin string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(map[string]any{})
	req := httptest.NewRequest(http.MethodPost, "/api/system/shutdown", &buf)
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// TestSystemShutdown_OriginGuard 跨站表单防护：外部 Origin → 403 且不触发排空；
// 同源 Origin → 202；无 Origin（curl/CLI）→ 202。
func TestSystemShutdown_OriginGuard(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		origin string
		want   int
	}{
		{name: "cross-site origin rejected", host: "127.0.0.1:8080", origin: "http://evil.example", want: http.StatusForbidden},
		{name: "same-origin accepted", host: "127.0.0.1:8080", origin: "http://127.0.0.1:8080", want: http.StatusAccepted},
		{name: "no origin passes", host: "127.0.0.1:8080", origin: "", want: http.StatusAccepted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := newAPIServerWithMock(t, "drain-api", 1, true)
			r := srv.Router()
			startAPIManager(t, srv)

			rr := doShutdownPost(t, r, tt.host, tt.origin)
			if rr.Code != tt.want {
				t.Fatalf("POST /api/system/shutdown (host=%q origin=%q) returned %d, want %d; body=%s",
					tt.host, tt.origin, rr.Code, tt.want, rr.Body.String())
			}
			// 拒绝的请求绝不能触发排空；放行的请求必须已进入 drain。
			if tt.want == http.StatusForbidden && srv.mgr.DrainRequested() {
				t.Fatal("cross-origin request must not trigger drain")
			}
			if tt.want == http.StatusAccepted && !srv.mgr.DrainRequested() {
				t.Fatal("same-origin/no-origin request should trigger drain")
			}
		})
	}
}
