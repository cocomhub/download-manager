// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
	"github.com/cocomhub/download-manager/testutil/assert"
)

// newMediaServer 构造一个带固定 cover 字段的 mock 任务（对象 ID=0）。
func newMediaServer(t *testing.T, coverPath string) *Server {
	t.Helper()
	cfg := &config.Config{
		Runtime: config.Runtime{
			Mode: config.RunModeFull,
			Download: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{Enabled: true},
			Scheduler: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{Enabled: true},
		},
		Server: config.Server{
			WorkDir:         t.TempDir(),
			DownloadRootDir: t.TempDir(),
		},
		Downloader: config.Downloader{GlobalConcurrent: 5, MaxRetries: 2},
		Tasks: []config.Task{{
			ID:      "mock-media",
			Type:    "mock",
			SaveDir: t.TempDir(),
			Storage: config.StorageConfig{Type: "memory"},
			Extra: map[string]any{
				"mock_rules": []any{map[string]any{
					"url_template": "http://mock-download/file-{n}.bin",
					"count":        1,
					"extra": map[string]any{
						"cover_url":  "https://example.com/cover.jpg",
						"cover_path": coverPath,
					},
				}},
				"refresh_interval": 0,
			},
		}},
	}
	return NewServer(manager.NewManager(cfg))
}

func waitForMediaSeed(t *testing.T, r http.Handler, taskID string) {
	t.Helper()
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/"+taskID)
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for mock task to seed objects")
}

func TestAPI_GetObjectMedia_ServesLocalCover(t *testing.T) {
	coverPath := filepath.Join(t.TempDir(), "cover.jpg")
	if err := os.WriteFile(coverPath, []byte("fake-image-bytes"), 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}
	srv := newMediaServer(t, coverPath)
	r := srv.Router()
	done := startAPIManager(t, srv)
	waitForMediaSeed(t, r, "mock-media")

	rr := doJSONGet(t, r, "/api/objects/mock/0/media/cover")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET media/cover returned %d, want 200: %s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != "fake-image-bytes" {
		t.Errorf("body = %q, want fake-image-bytes", rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", ct)
	}
	_ = done
}

func TestAPI_GetObjectMedia_RedirectsWhenNoLocalPath(t *testing.T) {
	// 无 cover_path（仅 cover_url）→ 302 到源 URL
	srv := newMediaServer(t, "")
	r := srv.Router()
	done := startAPIManager(t, srv)
	waitForMediaSeed(t, r, "mock-media")

	rr := doJSONGet(t, r, "/api/objects/mock/0/media/cover")
	if rr.Code != http.StatusFound {
		t.Fatalf("GET media/cover returned %d, want 302", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "https://example.com/cover.jpg" {
		t.Errorf("Location = %q, want https://example.com/cover.jpg", loc)
	}
	_ = done
}

func TestAPI_GetObjectMedia_InvalidRelAndMissingMedia(t *testing.T) {
	srv := newMediaServer(t, "")
	r := srv.Router()
	done := startAPIManager(t, srv)
	waitForMediaSeed(t, r, "mock-media")

	// 非法 rel → 400
	if rr := doJSONGet(t, r, "/api/objects/mock/0/media/poster"); rr.Code != http.StatusBadRequest {
		t.Errorf("invalid rel returned %d, want 400", rr.Code)
	}
	// 未设置的媒体类型 → 404
	if rr := doJSONGet(t, r, "/api/objects/mock/0/media/thumb"); rr.Code != http.StatusNotFound {
		t.Errorf("missing thumb returned %d, want 404", rr.Code)
	}
	// 不存在的对象 → 404
	if rr := doJSONGet(t, r, "/api/objects/mock/99999/media/cover"); rr.Code != http.StatusNotFound {
		t.Errorf("unknown id returned %d, want 404", rr.Code)
	}
	_ = done
}
