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

func TestWriteGuardUIMode(t *testing.T) {
	mgr := newMgrWithMode(config.RunModeUI, false, false)
	srv := NewServer(mgr)
	r := srv.Router()

	targets := []string{
		"/api/tasks/t1/retry",
		"/api/tasks/t1/cancel",
		"/api/tasks/cancel_batch",
		"/api/tasks/t1/object/cancel",
		"/api/tasks/t1/object/undo_cancel",
	}
	for _, url := range targets {
		rr := doJSONPost(r, url, map[string]any{"ids": []string{"t1"}, "url": "http://a"})
		if rr.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for %s, got %d", url, rr.Code)
		}
		var resp map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &resp)
		if resp["error"] != "write operations disabled in ui-only mode" {
			t.Fatalf("unexpected error payload for %s: %v", url, resp)
		}
	}
}

func TestWriteGuardFullMode_Allows(t *testing.T) {
	mgr := newMgrWithMode(config.RunModeFull, true, true)
	srv := NewServer(mgr)
	r := srv.Router()

	// cancel_batch should not be blocked by guard and returns 200 with result map
	rr := doJSONPost(r, "/api/tasks/cancel_batch", map[string]any{"ids": []string{"t1"}})
	if rr.Code < 200 || rr.Code >= 300 {
		t.Fatalf("expected 2xx for cancel_batch in full mode, got %d", rr.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
}
