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

// TestAPI_GetObjectByTypeAndID_Found verifies GET /api/objects/{type}/{id} returns a found object.
func TestAPI_GetObjectByTypeAndID_Found(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-obj-found", 1, false)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-obj-found")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for task to seed objects")

	// Mock objects all have ID=0, so query with ID=0.
	rr := doJSONGet(t, r, "/api/objects/mock/0")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/objects/mock/0 returned %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var obj map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &obj); err != nil {
		t.Fatalf("unmarshal object: %v", err)
	}
	if obj["id"] == nil {
		t.Error("expected id field in response")
	}
	if obj["task_id"] != "mock-obj-found" {
		t.Errorf("task_id = %v, want mock-obj-found", obj["task_id"])
	}

	_ = done
}

// TestAPI_GetObjectByTypeAndID_NotFoundType verifies 404 for unknown task type.
func TestAPI_GetObjectByTypeAndID_NotFoundType(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-obj-type404", 1, false)
	r := srv.Router()

	rr := doJSONGet(t, r, "/api/objects/nonexistent/0")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET /api/objects/nonexistent/0 returned %d, want 404", rr.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["error"] != "not_found" {
		t.Errorf("error code = %q, want not_found", resp["error"])
	}
}

// TestAPI_GetObjectByTypeAndID_NotFoundID verifies 404 for unknown object ID.
func TestAPI_GetObjectByTypeAndID_NotFoundID(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-obj-id404", 1, false)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-obj-id404")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for task to seed objects")

	// ID=99999 should not exist.
	rr := doJSONGet(t, r, "/api/objects/mock/99999")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET /api/objects/mock/99999 returned %d, want 404", rr.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["error"] != "not_found" {
		t.Errorf("error code = %q, want not_found", resp["error"])
	}

	_ = done
}

// TestAPI_GetObjectByTypeAndID_InvalidID verifies 400 for non-positive integer ID.
func TestAPI_GetObjectByTypeAndID_InvalidID(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-obj-invalid", 1, false)
	r := srv.Router()

	rr := doJSONGet(t, r, "/api/objects/mock/abc")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("GET /api/objects/mock/abc returned %d, want 400", rr.Code)
	}

	rr2 := doJSONGet(t, r, "/api/objects/mock/-1")
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("GET /api/objects/mock/-1 returned %d, want 400", rr2.Code)
	}

	rr3 := doJSONGet(t, r, "/api/objects/mock/0")
	if rr3.Code != http.StatusOK {
		t.Fatalf("GET /api/objects/mock/0 returned %d, want 200 (ID=0 is valid)", rr3.Code)
	}
}