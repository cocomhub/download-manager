// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
)

func TestGetGroupObjects_RequiresTaskScope(t *testing.T) {
	mgr := manager.NewManager(&config.Config{})
	srv := NewServer(mgr)
	req := httptest.NewRequest(http.MethodGet, "/api/groups/CLUB-100/objects", nil)
	rr := httptest.NewRecorder()

	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expect 400, got %d", rr.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if resp["error"] != "missing_scope" {
		t.Fatalf("unexpected error code: %+v", resp)
	}
}

func TestGetGroupObjects_SuccessPath(t *testing.T) {
	srv, _ := newAPIServerWithMockWithGroup(t, "mock-group-succ", 3, "test-group-1")
	r := srv.Router()

	done := startAPIManager(t, srv)
	time.Sleep(500 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/api/groups/test-group-1/objects?task_id=mock-group-succ&task_type=mock", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expect 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["group"] != "test-group-1" {
		t.Errorf("expected group=test-group-1, got %v", result["group"])
	}
	if result["task_id"] != "mock-group-succ" {
		t.Errorf("expected task_id=mock-group-succ, got %v", result["task_id"])
	}
	if result["task_type"] != "mock" {
		t.Errorf("expected task_type=mock, got %v", result["task_type"])
	}
	total, _ := result["total"].(float64)
	t.Logf("got total=%v (expected 3)", total)
	if total < 3 {
		t.Fatalf("expected total >= 3, got %v", total)
	}

	_ = done
}

// newAPIServerWithMockWithGroup creates a server with mock objects that have
// the given content_group metadata.
func newAPIServerWithMockWithGroup(t *testing.T, taskID string, objectCount int, group string) (*Server, *config.Config) {
	t.Helper()

	cfg := &config.Config{
		Runtime: config.Runtime{
			Mode: config.RunModeFull,
			Download: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{
				Enabled: true,
			},
			Scheduler: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{
				Enabled: true,
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
							"url_template": "http://mock-download/group-{n}.bin",
							"count":        objectCount,
							"metadata": map[string]any{
								"content_group": group,
							},
						},
					},
				},
			},
		},
	}

	mgr := manager.NewManager(cfg)
	srv := NewServer(mgr)
	return srv, cfg
}
