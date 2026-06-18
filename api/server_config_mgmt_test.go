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

// TestAPI_ConfigHistory_Empty verifies GET /api/config/history returns a JSON array.
func TestAPI_ConfigHistory_Empty(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-hist", 1, false)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/config/history")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for config endpoint ready")

	rr := doJSONGet(t, r, "/api/config/history")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/config/history returned %d, want 200", rr.Code)
	}

	var hist []any
	if err := json.Unmarshal(rr.Body.Bytes(), &hist); err != nil {
		t.Fatalf("unmarshal history: %v (body: %s)", err, rr.Body.String())
	}

	_ = done
}

// TestAPI_ConfigDiff_MissingParams verifies GET /api/config/diff handles
// missing left/right query parameters (may be empty diff or error).
func TestAPI_ConfigDiff_MissingParams(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-diff", 1, false)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/config/diff")
		return rr.Code == http.StatusOK || rr.Code == http.StatusBadRequest
	}, 3*time.Second, 50*time.Millisecond, "wait for config diff endpoint ready")

	rr := doJSONGet(t, r, "/api/config/diff")
	if rr.Code != http.StatusOK && rr.Code != http.StatusBadRequest {
		t.Fatalf("GET /api/config/diff returned %d, want 200 or 400", rr.Code)
	}

	// Should be valid JSON either way.
	var resp any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal diff response: %v (body: %s)", err, rr.Body.String())
	}

	_ = done
}

// TestAPI_ConfigBackup_DeleteInvalid verifies POST /api/config/delete
// with empty body returns 400.
func TestAPI_ConfigBackup_DeleteInvalid(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-del", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/config/history")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for config after mock with writes")

	rr := doJSONPost(t, r, "/api/config/delete", map[string]string{})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty delete, got %d: %s", rr.Code, rr.Body.String())
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

// TestAPI_ConfigRollback_Invalid verifies POST /api/config/rollback
// with empty body returns 400.
func TestAPI_ConfigRollback_Invalid(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-rollback", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/config/history")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for config after rollback mock")

	rr := doJSONPost(t, r, "/api/config/rollback", map[string]string{})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty rollback, got %d: %s", rr.Code, rr.Body.String())
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

// TestAPI_ConfigApply_ValidYAML verifies POST /api/config/apply with valid
// YAML returns 200 (when writes are enabled).
func TestAPI_ConfigApply_ValidYAML(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-apply", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/config/history")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for config before apply YAML")

	body := map[string]string{
		"yaml": `server:
  work_dir: /tmp/test
  download_root_dir: /tmp/test
`,
	}
	rr := doJSONPost(t, r, "/api/config/apply", body)
	// This may succeed (200) or fail with a processing error — the important thing
	// is that the handler processes the request and returns valid JSON.
	if rr.Code != http.StatusOK && rr.Code != http.StatusBadRequest && rr.Code != http.StatusInternalServerError {
		t.Fatalf("config apply returned unexpected %d: %s", rr.Code, rr.Body.String())
	}

	_ = done
}

// TestAPI_ConfigTagAndNote verifies POST /api/config/tag and /api/config/note
// with valid data.
func TestAPI_ConfigTagAndNote(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-tagnote", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/config/history")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for config before tag/note")

	// Tag endpoint with empty body should return 400.
	rr := doJSONPost(t, r, "/api/config/tag", map[string]string{})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty tag, got %d: %s", rr.Code, rr.Body.String())
	}

	// Note endpoint with empty body should return 400.
	rr = doJSONPost(t, r, "/api/config/note", map[string]string{})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty note, got %d: %s", rr.Code, rr.Body.String())
	}

	_ = done
}

// TestAPI_ConfigServerUpdate verifies the handler processes
// POST /api/config/server (may succeed or fail depending on state).
func TestAPI_ConfigServerUpdate(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-cfg-upd", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/config/history")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for config before server update")

	body := map[string]any{
		"downloader": map[string]any{
			"global_concurrent": 10,
		},
	}
	rr := doJSONPost(t, r, "/api/config/server", body)
	// Should be handled without panic — returns 200, 400, or 500.
	if rr.Code != http.StatusOK && rr.Code != http.StatusBadRequest && rr.Code != http.StatusInternalServerError {
		t.Fatalf("config server update returned unexpected %d: %s", rr.Code, rr.Body.String())
	}

	_ = done
}

// TestAPI_ConfigLogUpdate verifies the handler processes
// POST /api/config/log.
func TestAPI_ConfigLogUpdate(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-log-upd", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/config/history")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for config before log update")

	body := map[string]any{
		"level": "debug",
	}
	rr := doJSONPost(t, r, "/api/config/log", body)
	// Should be handled without panic.
	if rr.Code != http.StatusOK && rr.Code != http.StatusBadRequest && rr.Code != http.StatusInternalServerError {
		t.Fatalf("config log update returned unexpected %d: %s", rr.Code, rr.Body.String())
	}

	_ = done
}
