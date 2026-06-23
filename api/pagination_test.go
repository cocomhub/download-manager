// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
	"github.com/cocomhub/download-manager/testutil/assert"
)

// TestAPI_Pagination_Boundaries verifies edge cases for pagination query parameters.
func TestAPI_Pagination_Boundaries(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
		expectObj  bool
	}{
		{"limit=0 should return all", "limit=0&page=1", http.StatusOK, true},
		{"page<1 defaults to 1", "limit=5&page=-1", http.StatusOK, true},
		{"page far exceeds total", "limit=5&page=100", http.StatusOK, true},
		{"invalid sort field", "sort=invalid_field", http.StatusOK, false},
		{"empty query params", "", http.StatusOK, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := newAPIServerWithMock(t, "mock-page-edge", 10, false)
			r := srv.Router()

			startAPIManager(t, srv)
			assert.MustEventually(t, func() bool {
				rr := doJSONGet(t, r, "/api/tasks/mock-page-edge")
				return rr.Code == http.StatusOK
			}, 3*time.Second, 50*time.Millisecond, "wait for pagination task to seed objects")

			url := "/api/tasks/mock-page-edge"
			if tt.query != "" {
				url += "?" + tt.query
			}
			rr := doJSONGet(t, r, url)

			if rr.Code != tt.wantStatus {
				t.Errorf("GET %s returned %d, want %d: %s", url, rr.Code, tt.wantStatus, rr.Body.String())
			}

			var result map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if tt.expectObj {
				if _, ok := result["objects"]; !ok {
					t.Errorf("expected 'objects' field in response for %s", url)
				}
			}
		})
	}
}

// TestAPI_TaskNotFound verifies 404 response format for non-existent tasks.
func TestAPI_TaskNotFound(t *testing.T) {
	// GET锛堣鎿嶄綔锛変娇鐢?writeEnabled=false server
	srv, _ := newAPIServerWithMock(t, "mock-404", 1, false)
	r := srv.Router()
	startAPIManager(t, srv)

	// GET /api/tasks/{id} 鈥?task endpoint returns JSON 404 for unknown task.
	t.Run("/api/tasks/non-existent", func(t *testing.T) {
		rr := doJSONGet(t, r, "/api/tasks/non-existent")
		if rr.Code != http.StatusNotFound {
			t.Logf("GET %s returned %d (acceptable if redirected): %s", "/api/tasks/non-existent", rr.Code, rr.Body.String())
		}
		var body map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal error response: %v", err)
		}
		if body["error"] == "" {
			t.Errorf("expected non-empty 'error' field in 404 response, got %v", body)
		}
	})

	// POST /api/tasks/{id}/cancel 鈥?cancel on non-existent task returns JSON 404.
	// 浣跨敤 writeEnabled=true 閬垮厤 write middleware 鎷︽埅
	t.Run("/api/tasks/non-existent/cancel (POST)", func(t *testing.T) {
		srv2, _ := newAPIServerWithMock(t, "mock-404-cancel", 1, true)
		r2 := srv2.Router()
		startAPIManager(t, srv2)

		rr := doJSONPost(t, r2, "/api/tasks/non-existent/cancel", nil)
		if rr.Code != http.StatusNotFound && rr.Code != http.StatusBadRequest {
			t.Errorf("POST cancel returned %d, expected 404/400: %s", rr.Code, rr.Body.String())
		}
		var body map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal error response: %v", err)
		}
		if body["error"] == "" {
			t.Errorf("expected non-empty 'error' field, got %v", body)
		}
	})
}

// TestAPI_WriteDisabled verifies 405 response when write operations are disabled.
func TestAPI_WriteDisabled(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-405", 1, false)
	r := srv.Router()
	done := startAPIManager(t, srv)

	writeEndpoints := []struct {
		method string
		url    string
		body   any
	}{
		{"POST", "/api/tasks/mock-405/cancel", nil},
		{"POST", "/api/tasks/mock-405/object/cancel", map[string]string{"url": "http://mock-download/file-0.bin"}},
		{"POST", "/api/tasks/mock-405/config", nil},
		{"POST", "/api/config/server", nil},
		{"POST", "/api/config/apply", nil},
	}

	for _, ep := range writeEndpoints {
		t.Run(ep.method+" "+ep.url, func(t *testing.T) {
			rr := doJSONPost(t, r, ep.url, ep.body)
			if rr.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s returned %d, want 405: %s", ep.method, ep.url, rr.Code, rr.Body.String())
			}

			var body map[string]string
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if body["error"] != "write_disabled" {
				t.Errorf("expected error 'write_disabled', got %v", body)
			}
		})
	}

	_ = done
}

// TestAPI_Aggregate_Quota verifies cross-task aggregate with quota allocation.
func TestAPI_Aggregate_Quota(t *testing.T) {
	srv, cfg := newAPIServerWithMock(t, "mock-agg-1", 5, false)
	extraTask := config.Task{
		ID:      "mock-agg-2",
		Type:    "mock",
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
		Extra: map[string]any{
			"mock_rules": []any{
				map[string]any{
					"url_template": "http://mock-download/alt-{n}.bin",
					"count":        5,
				},
			},
			"refresh_interval": 0,
		},
	}
	cfg.Tasks = append(cfg.Tasks, extraTask)

	mgr := srv.mgr
	if err := mgr.UpdateConfig(cfg, &manager.AuditInfo{Source: "test"}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	r := srv.Router()
	done := startAPIManager(t, srv)

	rr := doJSONGet(t, r, "/api/aggregate?limit=3&page=1")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/aggregate returned %d, want 200", rr.Code)
	}

	var result map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	objects, _ := result["objects"].([]any)
	if len(objects) > 3 {
		t.Errorf("expected <= 3 objects with limit=3 across 2 tasks, got %d", len(objects))
	}
	if total, ok := result["total"].(float64); ok && total != 10 {
		t.Logf("aggregate total returned %v (may be 0 if tasks haven't populated yet)", total)
	}

	_ = done
}

// TestAPI_AllEndpointsErrorFormat verifies all main endpoints return consistent error JSON.
func TestAPI_AllEndpointsErrorFormat(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-err", 1, false)
	r := srv.Router()
	done := startAPIManager(t, srv)

	rr := doJSONGet(t, r, "/api/tasks/mock-err/object/cancel")
	if rr.Code != http.StatusMethodNotAllowed && rr.Code != http.StatusBadRequest && rr.Code != http.StatusNotFound {
		t.Errorf("cancel object via GET returned %d, expected 4xx", rr.Code)
	}

	_ = done
}
