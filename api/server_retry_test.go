// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/testutil/assert"
)

// TestAPI_RetryObject verifies POST /api/tasks/{id}/retry with a specific URL.
func TestAPI_RetryObject(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-retry-obj", 2, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-retry-obj")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for mock-retry-obj task objects to be ready")

	// Cancel an object first so there's something to retry.
	// The cancel may race with resolve worker writing back pending — retry until it sticks.
	body := map[string]string{"url": "http://mock-download/file-0.bin"}
	assert.MustEventually(t, func() bool {
		rr := doJSONPost(t, r, "/api/tasks/mock-retry-obj/object/cancel", body)
		if rr.Code != http.StatusOK {
			return false
		}
		// Confirm object is now cancelled.
		crr := doJSONGet(t, r, "/api/tasks/mock-retry-obj")
		if crr.Code != http.StatusOK {
			return false
		}
		var result map[string]any
		if err := json.Unmarshal(crr.Body.Bytes(), &result); err != nil {
			return false
		}
		objects, _ := result["objects"].([]any)
		for _, raw := range objects {
			obj, _ := raw.(map[string]any)
			if obj == nil {
				continue
			}
			if obj["url"] == "http://mock-download/file-0.bin" {
				return obj["status"] == "cancelled"
			}
		}
		return false
	}, 3*time.Second, 50*time.Millisecond, "wait for object to be cancelled")

	// Retry the cancelled object.
	cancelResult := doJSONPost(t, r, "/api/tasks/mock-retry-obj/retry", body)
	if cancelResult.Code != http.StatusOK {
		t.Fatalf("retry object returned %d, want 200: %s", cancelResult.Code, cancelResult.Body.String())
	}

	_ = done
}

// TestAPI_RetryAllFailed verifies POST /api/tasks/{id}/retry without a URL
// retries all failed objects.
func TestAPI_RetryAllFailed(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-retry-all", 2, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-retry-all")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for mock-retry-all task objects to be ready")

	// Cancel an object first to have something retriable.
	body := map[string]string{"url": "http://mock-download/file-0.bin"}
	rr := doJSONPost(t, r, "/api/tasks/mock-retry-all/object/cancel", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("cancel object returned %d, want 200", rr.Code)
	}

	// Retry all (no body / empty body).
	rr = doJSONPost(t, r, "/api/tasks/mock-retry-all/retry", nil)
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
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-retry-404")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for mock-retry-404 task ready")

	body := map[string]string{"url": "http://mock-download/nonexistent.bin"}
	rr := doJSONPost(t, r, "/api/tasks/mock-retry-404/retry", body)
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
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-undo")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for mock-undo task objects to be ready")

	// Cancel first.
	body := map[string]string{"url": "http://mock-download/file-0.bin"}
	rr := doJSONPost(t, r, "/api/tasks/mock-undo/object/cancel", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("cancel object returned %d, want 200", rr.Code)
	}

	// Wait for the cancel to take effect (object status becomes cancelled).
	// The cancel may race with a download in flight that overwrites status back
	// to downloading; keep retrying the cancel until it sticks.
	assert.MustEventually(t, func() bool {
		crr := doJSONGet(t, r, "/api/tasks/mock-undo")
		if crr.Code != http.StatusOK {
			return false
		}
		var result map[string]any
		if err := json.Unmarshal(crr.Body.Bytes(), &result); err != nil {
			return false
		}
		objects, ok := result["objects"].([]any)
		if !ok || len(objects) == 0 {
			return false
		}
		// Check if any object has url=http://mock-download/file-0.bin and
		// is cancelled. If the cancel didn't stick (race with download),
		// re-issue the cancel.
		for _, raw := range objects {
			obj, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			url, _ := obj["url"].(string)
			if url == "http://mock-download/file-0.bin" {
				status, _ := obj["status"].(string)
				if status == "cancelled" {
					return true
				}
				// Re-cancel if status was overwritten by download.
				crr2 := doJSONPost(t, r, "/api/tasks/mock-undo/object/cancel", body)
				return crr2.Code == http.StatusOK
			}
		}
		return false
	}, 3*time.Second, 50*time.Millisecond, "wait for object to be cancelled")

	// Undo the cancel.
	rr = doJSONPost(t, r, "/api/tasks/mock-undo/object/undo_cancel", body)
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
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-undo-fail")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for mock-undo-fail task ready")

	rr := doJSONPost(t, r, "/api/tasks/mock-undo-fail/object/undo_cancel", map[string]string{})
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
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-undo-batch")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for mock-undo-batch task objects to be ready")

	// Cancel two objects and wait for both to be confirmed cancelled.
	for _, u := range []string{"http://mock-download/file-0.bin", "http://mock-download/file-1.bin"} {
		body := map[string]string{"url": u}
		rr := doJSONPost(t, r, "/api/tasks/mock-undo-batch/object/cancel", body)
		if rr.Code != http.StatusOK {
			t.Fatalf("cancel %s returned %d", u, rr.Code)
		}
	}
	assert.MustEventually(t, func() bool {
		crr := doJSONGet(t, r, "/api/tasks/mock-undo-batch")
		if crr.Code != http.StatusOK {
			return false
		}
		var result map[string]any
		if err := json.Unmarshal(crr.Body.Bytes(), &result); err != nil {
			return false
		}
		objects, _ := result["objects"].([]any)
		cancelled := 0
		for _, raw := range objects {
			obj, _ := raw.(map[string]any)
			if obj == nil {
				continue
			}
			url, _ := obj["url"].(string)
			if url != "http://mock-download/file-0.bin" && url != "http://mock-download/file-1.bin" {
				continue
			}
			status, _ := obj["status"].(string)
			if status == "cancelled" {
				cancelled++
			} else {
				// Re-cancel if overwritten by download race.
				doJSONPost(t, r, "/api/tasks/mock-undo-batch/object/cancel", map[string]string{"url": url})
			}
		}
		return cancelled >= 2
	}, 3*time.Second, 50*time.Millisecond, "wait for both objects to be cancelled")

	// Batch undo.
	rr := doJSONPost(t, r, "/api/tasks/mock-undo-batch/object/undo_cancel_batch", map[string][]string{
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
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-cancel-batch")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for mock-cancel-batch task objects to be ready")

	// Cancel objects via batch endpoint — race with resolve worker may
	// cause cancel to fail (object not found/not downloading), retry.
	assert.MustEventually(t, func() bool {
		rr := doJSONPost(t, r, "/api/tasks/mock-cancel-batch/object/cancel_batch", map[string][]string{
			"urls": {"http://mock-download/file-0.bin"},
		})
		if rr.Code != http.StatusOK {
			return false
		}
		var result map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			return false
		}
		return result["http://mock-download/file-0.bin"] == "ok"
	}, 3*time.Second, 50*time.Millisecond, "wait for cancel_batch to succeed on file-0")

	_ = done
}

// TestAPI_CancelTasksBatch verifies batch cancel of tasks.
func TestAPI_CancelTasksBatch(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-cancel-tasks-batch", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-cancel-tasks-batch")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for mock-cancel-tasks-batch task ready")

	rr := doJSONPost(t, r, "/api/tasks/cancel_batch", map[string][]string{
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
