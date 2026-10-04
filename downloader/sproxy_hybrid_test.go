// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
)

// TestSproxyHybrid_PickShareURL 验证分享 URL 提取（files[0] keepshare 优先，其次 obj.URL）。
func TestSproxyHybrid_PickShareURL(t *testing.T) {
	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{})
	obj := &model.DownloadObject{
		URL: "https://njavtv.com/dm102/ja/mism-099",
		Extra: map[string]any{
			"files": []map[string]string{
				{"url": "https://keepshare.org/abc/magnet:?xt=urn:btih:xyz"},
			},
		},
	}
	got := d.pickShareURL(obj)
	if !strings.Contains(got, "keepshare.org") {
		t.Fatalf("pickShareURL = %q, want keepshare", got)
	}

	// obj.URL 本身是分享
	obj2 := &model.DownloadObject{URL: "https://mypikpak.com/s/abc123"}
	if got := d.pickShareURL(obj2); got != obj2.URL {
		t.Fatalf("pickShareURL direct = %q, want %q", got, obj2.URL)
	}

	// 非分享 URL → 空
	obj3 := &model.DownloadObject{URL: "https://example.com/x.mp4"}
	if got := d.pickShareURL(obj3); got != "" {
		t.Fatalf("non-share URL should be empty, got %q", got)
	}
}

// TestSproxyHybrid_Download 端到端：提交 → 轮询 → 完成（fake sproxy API）。
func TestSproxyHybrid_Download(t *testing.T) {
	var submitted atomic.Bool
	done := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		submitted.Store(true)
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["url"] != "https://keepshare.org/abc/magnet:?xt=urn:btih:xyz" {
			t.Errorf("submit url = %q", body["url"])
		}
		writeJSONResp(w, map[string]any{"id": "task-1", "status": "running"})
	})
	mux.HandleFunc("GET /api/cloud/tasks/task-1", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-1", "status": "completed"})
		close(done)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{
		APIURL:    srv.URL + "/api/cloud/download",
		PollEvery: 100,
	})
	obj := &model.DownloadObject{
		URL:      "https://njavtv.com/dm102/ja/mism-099",
		SavePath: filepath.Join(t.TempDir(), "out.mp4"),
		Extra: map[string]any{
			"files": []map[string]string{
				{"url": "https://keepshare.org/abc/magnet:?xt=urn:btih:xyz"},
			},
		},
	}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download error: %v", err)
	}
	if !submitted.Load() {
		t.Error("task not submitted")
	}
}

// writeJSONResp 写 JSON 响应（测试 helper）。
func writeJSONResp(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
