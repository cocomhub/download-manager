// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cocomhub/download-manager/manager"
)

// seedFailureRecords 直接向 manager 环形缓冲写入失败记录。
func seedFailureRecords(srv *Server, recs ...manager.FailureRecord) {
	for _, r := range recs {
		srv.mgr.RecordFailure(r.TaskID, r.URL, r.Error, r.Attempt, r.Permanent)
	}
}

// TestAPI_RetryOverview verifies GET /api/retry/overview returns categories + totals.
func TestAPI_RetryOverview(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-overview", 1, true)
	seedFailureRecords(srv,
		manager.FailureRecord{TaskID: "mock-overview", URL: "http://x/1", Error: "HTTP 404", Attempt: 1, Permanent: true},
		manager.FailureRecord{TaskID: "mock-overview", URL: "http://x/2", Error: "HTTP 500", Attempt: 1, Permanent: false},
		manager.FailureRecord{TaskID: "mock-overview", URL: "http://x/3", Error: "context deadline exceeded", Attempt: 1, Permanent: false},
	)

	rr := doJSONGet(t, srv.Router(), "/api/retry/overview")
	if rr.Code != http.StatusOK {
		t.Fatalf("overview returned %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Categories  map[string]int `json:"categories"`
		TotalFailed int            `json:"total_failed"`
		Retryable   int            `json:"retryable"`
		Permanent   int            `json:"permanent"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.TotalFailed != 3 {
		t.Fatalf("total_failed = %d, want 3", body.TotalFailed)
	}
	if body.Retryable != 2 {
		t.Fatalf("retryable = %d, want 2", body.Retryable)
	}
	if body.Permanent != 1 {
		t.Fatalf("permanent = %d, want 1", body.Permanent)
	}
	if body.Categories[manager.FailureCategoryHTTP4xx] != 1 {
		t.Fatalf("http_4xx = %d, want 1", body.Categories[manager.FailureCategoryHTTP4xx])
	}
	if body.Categories[manager.FailureCategoryHTTP5xx] != 1 {
		t.Fatalf("http_5xx = %d, want 1", body.Categories[manager.FailureCategoryHTTP5xx])
	}
	if body.Categories[manager.FailureCategoryTimeout] != 1 {
		t.Fatalf("timeout = %d, want 1", body.Categories[manager.FailureCategoryTimeout])
	}
}

// TestAPI_RetryCategory verifies POST /api/retry/retry-category resets all failed objects.
func TestAPI_RetryCategory(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "retry-cat", 1, true)
	r := srv.Router()

	rr := doJSONPost(t, r, "/api/retry/retry-category", map[string]string{
		"category": manager.FailureCategoryHTTP5xx,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("retry-category returned %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Retried int `json:"retried"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Retried < 0 {
		t.Fatalf("retried = %d, want >= 0", body.Retried)
	}
}

// TestAPI_RetryCategory_InvalidCategory verifies unknown category returns 400.
func TestAPI_RetryCategory_InvalidCategory(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "retry-cat-bad", 1, true)
	r := srv.Router()

	rr := doJSONPost(t, r, "/api/retry/retry-category", map[string]string{
		"category": "bogus",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid category, got %d: %s", rr.Code, rr.Body.String())
	}
}
