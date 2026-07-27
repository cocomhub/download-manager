// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/testutil/assert"
)

func TestAPI_UpdateObjectTags_Success(t *testing.T) {
	t.Parallel()
	srv, _ := newAPIServerWithMock(t, "mock-object-tags", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)

	// Seed a test object with a known ID via the manager's storage directly.
	obj := &model.DownloadObject{
		TaskID: "mock-object-tags",
		URL:    "http://example.com/tags-test",
		Extra:  map[string]any{"tags": []string{"old1", "old2"}},
	}
	obj.SetID(1)
	task := srv.mgr.FirstTaskOfType("mock")
	if task == nil {
		t.Fatal("mock task not found")
	}
	if err := task.Storage().Update(obj); err != nil {
		t.Fatalf("failed to seed object: %v", err)
	}

	// Wait for the manager to be ready
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-object-tags")
		return rr.Code == http.StatusOK
	}, 3, 50, "wait for task endpoint ready")

	// POST tags
	body := map[string]any{"tags": []string{"new1", "new2", "new3"}}
	rr := doJSONPost(t, r, "/api/objects/mock/1/tags", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/objects/mock/1/tags returned %d, want 200: %s", rr.Code, rr.Body.String())
	}

	// Verify tags were updated via GET
	rr2 := doJSONGet(t, r, "/api/objects/mock/1")
	if rr2.Code != http.StatusOK {
		t.Fatalf("GET /api/objects/mock/1 returned %d, want 200", rr2.Code)
	}
	var result map[string]any
	if err := json.Unmarshal(rr2.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	extra, ok := result["extra"].(map[string]any)
	if !ok {
		t.Fatal("extra field missing or not a map")
	}
	tags, ok := extra["tags"].([]any)
	if !ok {
		t.Fatal("tags field missing or not a list")
	}
	if len(tags) != 3 || tags[0] != "new1" {
		t.Fatalf("tags = %v, want [new1 new2 new3]", tags)
	}

	_ = done
}

func TestAPI_UpdateObjectTags_EmptyTags(t *testing.T) {
	t.Parallel()
	srv, _ := newAPIServerWithMock(t, "mock-tags-empty", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)

	obj := &model.DownloadObject{
		TaskID: "mock-tags-empty",
		URL:    "http://example.com/tags-empty",
		Extra:  map[string]any{"tags": []string{"old1", "old2"}},
	}
	obj.SetID(1)
	task := srv.mgr.FirstTaskOfType("mock")
	if task == nil {
		t.Fatal("mock task not found")
	}
	if err := task.Storage().Update(obj); err != nil {
		t.Fatalf("failed to seed object: %v", err)
	}

	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-tags-empty")
		return rr.Code == http.StatusOK
	}, 3, 50, "wait for task endpoint ready")

	// Clear tags
	body := map[string]any{"tags": []string{}}
	rr := doJSONPost(t, r, "/api/objects/mock/1/tags", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST returned %d, want 200: %s", rr.Code, rr.Body.String())
	}

	_ = done
}

func TestAPI_UpdateObjectTags_InvalidBody(t *testing.T) {
	t.Parallel()
	srv, _ := newAPIServerWithMock(t, "mock-tags-invalid", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)

	// Empty body (not valid JSON)
	req := httptest.NewRequest("POST", "/api/objects/mock/1/tags", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("POST with empty body returned %d, want 400: %s", rr.Code, rr.Body.String())
	}

	_ = done
}

func TestAPI_UpdateObjectTags_ObjectNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := newAPIServerWithMock(t, "mock-tags-notfound", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)

	body := map[string]any{"tags": []string{"tag1"}}
	rr := doJSONPost(t, r, "/api/objects/mock/99999/tags", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("POST for non-existent object returned %d, want 400: %s", rr.Code, rr.Body.String())
	}

	_ = done
}

func TestAPI_UpdateObjectTags_InvalidID(t *testing.T) {
	t.Parallel()
	srv, _ := newAPIServerWithMock(t, "mock-tags-invalidid", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)

	body := map[string]any{"tags": []string{"tag1"}}
	rr := doJSONPost(t, r, "/api/objects/mock/abc/tags", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("POST with invalid ID returned %d, want 400: %s", rr.Code, rr.Body.String())
	}

	_ = done
}