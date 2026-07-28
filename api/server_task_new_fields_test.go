// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	_ "github.com/cocomhub/download-manager/task/mock" // register mock task type
	"github.com/cocomhub/download-manager/testutil/assert"
)

// TestAPI_PatchTaskRuntime_NewFields verifies that PATCH /api/tasks/{id}/runtime
// accepts scrape_enabled, download_enabled, and save_sub_dir fields.
func TestAPI_PatchTaskRuntime_NewFields(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-runtime", 2, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-runtime")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for mock-runtime task to be ready")

	t.Run("scrape_enabled_false", func(t *testing.T) {
		body := map[string]any{
			"scrape_enabled": false,
		}
		rr := doJSONPatch(t, r, "/api/tasks/mock-runtime/runtime", body)
		if rr.Code != http.StatusOK {
			t.Fatalf("PATCH runtime returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		applied, _ := resp["applied"].(map[string]any)
		if applied == nil {
			t.Fatal("response missing applied map")
		}
		if applied["scrape_enabled"] != true {
			t.Errorf("applied.scrape_enabled = %v, want true", applied["scrape_enabled"])
		}
	})

	t.Run("download_enabled_false", func(t *testing.T) {
		body := map[string]any{
			"download_enabled": false,
		}
		rr := doJSONPatch(t, r, "/api/tasks/mock-runtime/runtime", body)
		if rr.Code != http.StatusOK {
			t.Fatalf("PATCH runtime returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		applied, _ := resp["applied"].(map[string]any)
		if applied == nil {
			t.Fatal("response missing applied map")
		}
		if applied["download_enabled"] != true {
			t.Errorf("applied.download_enabled = %v, want true", applied["download_enabled"])
		}
	})

	t.Run("save_sub_dir", func(t *testing.T) {
		body := map[string]any{
			"save_sub_dir":     "videos/sub",
			"scrape_enabled":   true,
			"download_enabled": false,
		}
		rr := doJSONPatch(t, r, "/api/tasks/mock-runtime/runtime", body)
		if rr.Code != http.StatusOK {
			t.Fatalf("PATCH runtime returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		applied, _ := resp["applied"].(map[string]any)
		if applied == nil {
			t.Fatal("response missing applied map")
		}
		for _, key := range []string{"scrape_enabled", "download_enabled", "save_sub_dir"} {
			if applied[key] != true {
				t.Errorf("applied.%s = %v, want true", key, applied[key])
			}
		}
	})

	t.Run("all_fields", func(t *testing.T) {
		body := map[string]any{
			"concurrency":      5,
			"refresh_interval": 600,
			"scrape_enabled":   true,
			"download_enabled": false,
			"save_sub_dir":     "videos",
		}
		rr := doJSONPatch(t, r, "/api/tasks/mock-runtime/runtime", body)
		if rr.Code != http.StatusOK {
			t.Fatalf("PATCH runtime returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		applied, _ := resp["applied"].(map[string]any)
		if applied == nil {
			t.Fatal("response missing applied map")
		}
		for _, key := range []string{"concurrency", "refresh_interval", "scrape_enabled", "download_enabled", "save_sub_dir"} {
			if applied[key] != true {
				t.Errorf("applied.%s = %v, want true", key, applied[key])
			}
		}
	})

	_ = done
}

// doJSONPatch sends a PATCH request with JSON body.
func doJSONPatch(t *testing.T, router http.Handler, url string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest("PATCH", url, &buf)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// TestAPI_CreateTask_NewFields verifies that POST /api/tasks
// accepts scrape_enabled, download_enabled, and save_sub_dir fields.
func TestAPI_CreateTask_NewFields(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-create", 1, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	// Wait for the server to be ready
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for API ready")

	trueVal := true
	falseVal := false

	t.Run("create_with_new_fields", func(t *testing.T) {
		body := config.Task{
			ID:              "new-task-fields",
			Type:            "mock",
			SaveDir:         t.TempDir(),
			ScrapeEnabled:   &trueVal,
			DownloadEnabled: &falseVal,
			Storage:         config.StorageConfig{Type: "memory"},
			Extra: map[string]any{
				"mock_rules": []any{
					map[string]any{
						"url_template": "http://mock-download/new-file-{n}.bin",
						"count":        1,
					},
				},
			},
		}
		rr := doJSONPost(t, r, "/api/tasks", body)
		if rr.Code != http.StatusCreated {
			t.Fatalf("POST /api/tasks returned %d, want 201: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("read_back_new_fields", func(t *testing.T) {
		assert.MustEventually(t, func() bool {
			rr := doJSONGet(t, r, "/api/tasks/new-task-fields")
			return rr.Code == http.StatusOK
		}, 3*time.Second, 50*time.Millisecond, "wait for created task to be readable")

		rr := doJSONGet(t, r, "/api/tasks/new-task-fields")
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /api/tasks/new-task-fields returned %d, want 200", rr.Code)
		}
		var detail map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
			t.Fatalf("unmarshal detail: %v", err)
		}
		// Verify scrape_enabled
		se, ok := detail["scrape_enabled"].(bool)
		if !ok || !se {
			t.Errorf("scrape_enabled = %v, want true", detail["scrape_enabled"])
		}
		// Verify download_enabled
		de, ok := detail["download_enabled"].(bool)
		if !ok || de {
			t.Errorf("download_enabled = %v, want false", detail["download_enabled"])
		}
	})

	_ = done
}
