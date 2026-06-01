// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
)

func newMgrWithMode(mode config.RunMode, enableDownload, enableScheduler bool) *manager.Manager {
	cfg := &config.Config{
		Runtime: config.Runtime{
			Mode: mode,
			Download: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{Enabled: enableDownload},
			Scheduler: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{Enabled: enableScheduler},
		},
	}
	cfg.ValidateAndClamp()
	return manager.NewManager(cfg)
}

func doJSONPost(router http.Handler, url string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func doJSONPut(router http.Handler, url string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest("PUT", url, &buf)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func TestWriteGuardUIMode(t *testing.T) {
	mgr := newMgrWithMode(config.RunModeUI, false, false)
	srv := NewServer(mgr)
	r := srv.Router()

	targets := []string{
		// Task lifecycle
		"/api/tasks",
		"/api/tasks/t1/retry",
		"/api/tasks/t1/cancel",
		"/api/tasks/cancel_batch",

		// Object operations
		"/api/tasks/t1/object/cancel",
		"/api/tasks/t1/object/undo_cancel",
		"/api/tasks/t1/object/cancel_batch",
		"/api/tasks/t1/object/undo_cancel_batch",

		// Task reorder & config
		"/api/tasks/t1/reorder",
		"/api/tasks/t1/config",
		"/api/tasks/t1/runtime",

		// Server config
		"/api/config/server",
		"/api/config/log",
		"/api/config/rollback",
		"/api/config/tag",
		"/api/config/note",
		"/api/config/delete",
		"/api/config/apply",
	}
	for _, url := range targets {
		body := map[string]any{"ids": []string{"t1"}, "url": "http://a"}
		rr := doJSONPost(r, url, body)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 for POST %s, got %d", url, rr.Code)
		}
		var resp map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &resp)
		if resp["error"] != "write_disabled" {
			t.Fatalf("unexpected error code for POST %s: %v", url, resp)
		}
	}

	// PUT routes also covered
	rr := doJSONPut(r, "/api/tasks/t1", map[string]any{"id": "t1", "type": "test"})
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for PUT /api/tasks/t1, got %d", rr.Code)
	}
}

func TestWriteGuardFullMode_BlocksWhenBothDisabled(t *testing.T) {
	mgr := newMgrWithMode(config.RunModeFull, false, false)
	srv := NewServer(mgr)
	r := srv.Router()

	targets := []string{
		"/api/tasks",
		"/api/tasks/t1/retry",
		"/api/tasks/t1/cancel",
		"/api/tasks/t1/object/cancel",
		"/api/config/server",
		"/api/config/apply",
	}
	for _, url := range targets {
		rr := doJSONPost(r, url, map[string]any{"ids": []string{"t1"}, "url": "http://a"})
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 for %s in full mode with both disabled, got %d", url, rr.Code)
		}
	}
}

func TestWriteGuardFullMode_AllowsWhenOneEnabled(t *testing.T) {
	mgr := newMgrWithMode(config.RunModeFull, true, false)
	srv := NewServer(mgr)
	r := srv.Router()

	// With download enabled, writes should pass the guard
	// (they may still fail at the manager level due to missing state)
	rr := doJSONPost(r, "/api/tasks/t1/retry", map[string]any{"url": "http://a"})
	if rr.Code == http.StatusMethodNotAllowed {
		t.Fatalf("expected writes to pass guard when download enabled, got 405")
	}
}
