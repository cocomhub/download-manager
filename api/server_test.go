// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
	_ "github.com/cocomhub/download-manager/task/mock" // register mock task type
	"github.com/cocomhub/download-manager/testutil/assert"
)

// TestAPI_TaskList verifies GET /api/tasks returns registered mock tasks.
func TestAPI_TaskList(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-list", 2, false)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for manager to load tasks")

	rr := doJSONGet(t, r, "/api/tasks")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/tasks returned %d, want 200", rr.Code)
	}

	var tasks []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("unmarshal tasks: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatal("expected at least one task in list")
	}

	_ = done
}

// TestAPI_TaskDetail verifies GET /api/tasks/{id} returns task details.
func TestAPI_TaskDetail(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-detail", 3, false)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-detail")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for task detail endpoint ready")

	rr := doJSONGet(t, r, "/api/tasks/mock-detail")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/tasks/mock-detail returned %d, want 200", rr.Code)
	}

	var detail map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if id, ok := detail["id"].(string); !ok || id != "mock-detail" {
		t.Errorf("id = %v, want mock-detail", detail["id"])
	}

	_ = done
}

// TestAPI_TaskDetail_NotFound verifies 404 for unknown task.
func TestAPI_TaskDetail_NotFound(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-404", 1, false)
	r := srv.Router()

	rr := doJSONGet(t, r, "/api/tasks/nonexistent")
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown task, got %d", rr.Code)
	}

	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["error"] == "" {
		t.Error("expected error field in 404 response")
	}
}

// TestAPI_TaskDetail_WithPagination verifies page/limit parameters.
// Uses writeEnabled=true so the scheduler triggers object seeding
// before the API pagination query.
func TestAPI_TaskDetail_WithPagination(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-page", 10, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-page")
		if rr.Code != http.StatusOK {
			return false
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			return false
		}
		total, _ := result["total"].(float64)
		return total >= 10
	}, 3*time.Second, 50*time.Millisecond, "wait for 10 objects to be seeded")

	rr := doJSONGet(t, r, "/api/tasks/mock-page?limit=3&page=1")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/tasks/mock-page returned %d, want 200", rr.Code)
	}

	var result map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	objects, _ := result["objects"].([]any)
	if len(objects) > 3 {
		t.Errorf("expected <= 3 objects with limit=3, got %d", len(objects))
	}

	total := result["total"].(float64)
	if total != 10 {
		t.Errorf("total = %v, want 10 (objects=%v)", total, len(objects))
	}

	_ = done
}

// TestAPI_CancelObject verifies POST /api/tasks/{id}/object/cancel.
func TestAPI_CancelObject(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-cancel-api", 3, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-cancel-api")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for cancel api task ready")

	body := map[string]string{"url": "http://mock-download/file-0.bin"}
	assert.MustEventually(t, func() bool {
		rr := doJSONPost(t, r, "/api/tasks/mock-cancel-api/object/cancel", body)
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for cancel to succeed on mock-cancel-api")

	_ = done
}

// TestAPI_CancelObject_AlreadyCompleted verifies that canceling a completed
// object returns a specific error hinting at the delete feature.
func TestAPI_CancelObject_AlreadyCompleted(t *testing.T) {
	_, cfg := newAPIServerWithMock(t, "mock-cancel-completed", 1, true)
	// Override the task config: set initial status to completed.
	cfg.Tasks[0].Extra = map[string]any{
		"mock_rules": []any{
			map[string]any{
				"url_template": "http://mock-download/file-{n}.bin",
				"count":        1,
				"status":       "completed",
			},
		},
		"refresh_interval": 0,
	}
	// Recreate the manager with the updated config so objects start as completed.
	mgr := manager.NewManager(cfg)
	srv := NewServer(mgr)
	r := srv.Router()

	done := startAPIManager(t, srv)

	// Try to cancel the completed object 鈥?object seeding is lazy so retry
	// until the object exists in storage and we get the expected error.
	const wantHint = "use delete to remove it"
	assert.MustEventually(t, func() bool {
		body := map[string]string{"url": "http://mock-download/file-0.bin"}
		rr := doJSONPost(t, r, "/api/tasks/mock-cancel-completed/object/cancel", body)
		if rr.Code != http.StatusBadRequest {
			return false
		}
		var resp map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			return false
		}
		return strings.Contains(resp["message"], wantHint)
	}, 3*time.Second, 50*time.Millisecond, "wait for cancel to return delete hint on completed object")

	_ = done
}

// TestAPI_CancelTask verifies POST /api/tasks/{id}/cancel for a whole task.
func TestAPI_CancelTask(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-cancel-task", 2, true)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-cancel-task")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for cancel task ready")

	rr := doJSONPost(t, r, "/api/tasks/mock-cancel-task/cancel", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("cancel task returned %d, want 200: %s", rr.Code, rr.Body.String())
	}

	_ = done
}

// TestAPI_Metrics verifies GET /api/metrics returns health metrics.
func TestAPI_Metrics(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-metrics", 1, false)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/metrics")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for metrics endpoint ready")

	rr := doJSONGet(t, r, "/api/metrics")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/metrics returned %d, want 200", rr.Code)
	}

	var metrics map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &metrics); err != nil {
		t.Fatalf("unmarshal metrics: %v", err)
	}

	_ = done
}

// TestAPI_Aggregate verifies GET /api/aggregate returns aggregate data.
func TestAPI_Aggregate(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-agg", 2, false)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/aggregate")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for aggregate endpoint ready")

	rr := doJSONGet(t, r, "/api/aggregate")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/aggregate returned %d, want 200", rr.Code)
	}

	var agg map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &agg); err != nil {
		t.Fatalf("unmarshal aggregate: %v", err)
	}

	_ = done
}

// TestAPI_Health verifies GET /api/healthz returns health status.
func TestAPI_Health(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-health", 1, false)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/healthz")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for health endpoint ready")

	rr := doJSONGet(t, r, "/api/healthz")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/healthz returned %d, want 200", rr.Code)
	}

	var health map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &health); err != nil {
		t.Fatalf("unmarshal health: %v", err)
	}
	if health["status"] == nil {
		t.Error("expected status field in health response")
	}

	_ = done
}

// TestAPI_Config_Get verifies GET /api/config/server returns the config.
func TestAPI_Config_Get(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-cfg", 1, false)
	r := srv.Router()

	done := startAPIManager(t, srv)

	rr := doJSONGet(t, r, "/api/config/server")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/config/server returned %d, want 200", rr.Code)
	}

	var cfg map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	_ = done
}

// TestAPI_Runtime verifies GET /api/runtime returns runtime info.
func TestAPI_Runtime(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-rt", 1, false)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/runtime")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for runtime endpoint ready")

	rr := doJSONGet(t, r, "/api/runtime")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/runtime returned %d, want 200", rr.Code)
	}

	var runtime map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &runtime); err != nil {
		t.Fatalf("unmarshal runtime: %v", err)
	}

	_ = done
}

// TestAPI_Downloads verifies GET /api/downloads returns active downloads list.
func TestAPI_Downloads(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-dls", 1, false)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/downloads")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for downloads endpoint ready")

	rr := doJSONGet(t, r, "/api/downloads")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/downloads returned %d, want 200", rr.Code)
	}

	var downloads []any
	if err := json.Unmarshal(rr.Body.Bytes(), &downloads); err != nil {
		t.Fatalf("unmarshal downloads: %v", err)
	}

	_ = done
}

// TestAPI_Failures verifies GET /api/metrics/failures returns failure metrics.
func TestAPI_Failures(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-fail", 1, false)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/metrics/failures")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for failures endpoint ready")

	rr := doJSONGet(t, r, "/api/metrics/failures")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/metrics/failures returned %d, want 200", rr.Code)
	}

	var failures any
	if err := json.Unmarshal(rr.Body.Bytes(), &failures); err != nil {
		t.Fatalf("unmarshal failures: %v", err)
	}

	_ = done
}

// --- helpers ---

// newAPIServerWithMock creates an API server with a mock task.
// When writeEnabled is true, Runtime.Download.Enabled is set so that
// write operations (cancel, retry) are not blocked by the write guard.
func newAPIServerWithMock(t *testing.T, taskID string, objectCount int, writeEnabled bool) (*Server, *config.Config) {
	t.Helper()

	cfg := &config.Config{
		Runtime: config.Runtime{
			Mode: config.RunModeFull,
			Download: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{
				Enabled: writeEnabled,
			},
			Scheduler: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{
				Enabled: writeEnabled,
			},
		},
		Server: config.Server{
			WorkDir:         t.TempDir(),
			DownloadRootDir: t.TempDir(),
		},
		Downloader: config.Downloader{
			GlobalConcurrent: 5,
			MaxRetries:       2,
		},
		Tasks: []config.Task{
			{
				ID:      taskID,
				Type:    "mock",
				SaveDir: t.TempDir(),
				Storage: config.StorageConfig{Type: "memory"},
				Extra: map[string]any{
					"mock_rules": []any{
						map[string]any{
							"url_template": "http://mock-download/file-{n}.bin",
							"count":        objectCount,
						},
					},
					"refresh_interval": 0,
				},
			},
		},
	}

	mgr := manager.NewManager(cfg)
	srv := NewServer(mgr)
	return srv, cfg
}

// startAPIManager starts the manager in a goroutine and registers cleanup.
func startAPIManager(t *testing.T, srv *Server) chan struct{} {
	t.Helper()
	mgr := srv.mgr
	done := make(chan struct{})
	go func() {
		mgr.Start()
		close(done)
	}()

	// Wait for manager initialization to complete before returning.
	// This prevents data races when tests interact with the manager
	// while Start() is still loading tasks.
	<-mgr.Initialized()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		mgr.Stop(ctx)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	return done
}

func doJSONGet(t *testing.T, router http.Handler, url string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

// doJSONPost is defined in server_write_guard_test.go (same package).
