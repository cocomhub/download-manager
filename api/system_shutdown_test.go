// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
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
