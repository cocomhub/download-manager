// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestAPI_RetryObject verifies POST /api/tasks/{id}/retry with a specific URL.
func TestAPI_RetryObject(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-retry-obj", 2, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	time.Sleep(500 * time.Millisecond)

	// Cancel an object first so there's something to retry.
	body := map[string]string{"url": "http://mock-download/file-0.bin"}
	rr := doJSONPost(r, "/api/tasks/mock-retry-obj/object/cancel", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("cancel object returned %d, want 200: %s", rr.Code, rr.Body.String())
	}

	// Retry the cancelled object.
	rr = doJSONPost(r, "/api/tasks/mock-retry-obj/retry", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("retry object returned %d, want 200: %s", rr.Code, rr.Body.String())
	}

	_ = done
}

// TestAPI_RetryAllFailed verifies POST /api/tasks/{id}/retry without a URL
// retries all failed objects.
func TestAPI_RetryAllFailed(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-retry-all", 2, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	time.Sleep(500 * time.Millisecond)

	// Cancel an object first to have something retriable.
	body := map[string]string{"url": "http://mock-download/file-0.bin"}
	rr := doJSONPost(r, "/api/tasks/mock-retry-all/object/cancel", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("cancel object returned %d, want 200", rr.Code)
	}

	// Retry all (no body / empty body).
	rr = doJSONPost(r, "/api/tasks/mock-retry-all/retry", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("retry all returned %d, want 200: %s", rr.Code, rr.Body.String())
	}

	_ = done
}

// TestAPI_RetryObject_NotFound verifies retry with unknown URL returns 400.
func TestAPI_RetryObject_NotFound(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-retry-404", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	time.Sleep(200 * time.Millisecond)

	body := map[string]string{"url": "http://mock-download/nonexistent.bin"}
	rr := doJSONPost(r, "/api/tasks/mock-retry-404/retry", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for retry nonexistent, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("400 response body not JSON: %v (body: %s)", err, rr.Body.String())
	}
	if resp["error"] == "" {
		t.Error("expected error field in 400 response")
	}

	_ = done
}

// TestAPI_UndoCancelObject verifies POST /api/tasks/{id}/object/undo_cancel.
func TestAPI_UndoCancelObject(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-undo", 2, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	time.Sleep(500 * time.Millisecond)

	// Cancel first.
	body := map[string]string{"url": "http://mock-download/file-0.bin"}
	rr := doJSONPost(r, "/api/tasks/mock-undo/object/cancel", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("cancel object returned %d, want 200", rr.Code)
	}

	// Undo the cancel.
	rr = doJSONPost(r, "/api/tasks/mock-undo/object/undo_cancel", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("undo cancel returned %d, want 200: %s", rr.Code, rr.Body.String())
	}

	_ = done
}

// TestAPI_UndoCancelObject_MissingURL verifies undo_cancel without URL returns 400.
func TestAPI_UndoCancelObject_MissingURL(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-undo-fail", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	time.Sleep(200 * time.Millisecond)

	rr := doJSONPost(r, "/api/tasks/mock-undo-fail/object/undo_cancel", map[string]string{})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing url, got %d: %s", rr.Code, rr.Body.String())
	}

	_ = done
}

// TestAPI_UndoCancelObjectsBatch verifies batch undo cancel.
func TestAPI_UndoCancelObjectsBatch(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-undo-batch", 3, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	time.Sleep(500 * time.Millisecond)

	// Cancel two objects.
	for _, u := range []string{"http://mock-download/file-0.bin", "http://mock-download/file-1.bin"} {
		rr := doJSONPost(r, "/api/tasks/mock-undo-batch/object/cancel", map[string]string{"url": u})
		if rr.Code != http.StatusOK {
			t.Fatalf("cancel %s returned %d", u, rr.Code)
		}
	}

	// Batch undo.
	rr := doJSONPost(r, "/api/tasks/mock-undo-batch/object/undo_cancel_batch", map[string][]string{
		"urls": {"http://mock-download/file-0.bin", "http://mock-download/file-1.bin"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("batch undo returned %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var result map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["http://mock-download/file-0.bin"] != "ok" {
		t.Errorf("expected ok for file-0, got %s", result["http://mock-download/file-0.bin"])
	}

	_ = done
}

// TestAPI_CancelObjectsBatch verifies batch cancel of objects.
func TestAPI_CancelObjectsBatch(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-cancel-batch", 3, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	time.Sleep(500 * time.Millisecond)

	rr := doJSONPost(r, "/api/tasks/mock-cancel-batch/object/cancel_batch", map[string][]string{
		"urls": {"http://mock-download/file-0.bin", "http://mock-download/file-1.bin"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("batch cancel returned %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var result map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["http://mock-download/file-0.bin"] != "ok" {
		t.Errorf("expected ok for file-0, got %s", result["http://mock-download/file-0.bin"])
	}

	_ = done
}

// TestAPI_CancelTasksBatch verifies batch cancel of tasks.
func TestAPI_CancelTasksBatch(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-cancel-tasks-batch", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	time.Sleep(200 * time.Millisecond)

	rr := doJSONPost(r, "/api/tasks/cancel_batch", map[string][]string{
		"ids": {"mock-cancel-tasks-batch"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("batch cancel tasks returned %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var result map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["mock-cancel-tasks-batch"] == "" {
		t.Error("expected result entry for task")
	}

	_ = done
}
