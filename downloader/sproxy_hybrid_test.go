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
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
)

// TestSproxyHybrid_PickShareURL 验证分享 URL 提取。
// 优先级：magnet_list[].keepshare（与 gopeed collectPikPakCandidates 对齐）→ files[] → obj.URL。
func TestSproxyHybrid_PickShareURL(t *testing.T) {
	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{})

	// ① magnet_list[].keepshare 优先（keepshare 镜像 = HTTP 形态，hybrid 可用）
	obj := &model.DownloadObject{
		URL: "https://njavtv.com/dm102/ja/mism-099",
		Extra: map[string]any{
			"magnet_list": []map[string]string{
				{"name": "v.mp4", "keepshare": "https://keepshare.org/abc/magnet:?xt=urn:btih:xyz", "magnet": "magnet:?xt=urn:btih:xyz", "size": "4.65GB"},
			},
		},
	}
	if got := d.pickShareURL(obj); !strings.Contains(got, "keepshare.org") {
		t.Fatalf("magnet_list keepshare = %q, want keepshare", got)
	}

	// ①b magnet_list 里只有纯 magnet（无 keepshare）→ hybrid 用不了，返回空
	objML := &model.DownloadObject{
		Extra: map[string]any{
			"magnet_list": []map[string]string{
				{"name": "keep.mp4", "magnet": "magnet:?xt=urn:btih:yyy", "size": "2GB"},
			},
		},
	}
	if got := d.pickShareURL(objML); got != "" {
		t.Fatalf("pure magnet should be empty (hybrid can't use), got %q", got)
	}

	// ② files[] keepshare 分享链接
	obj3 := &model.DownloadObject{
		Extra: map[string]any{
			"files": []map[string]string{
				{"url": "https://keepshare.org/abc/magnet:?xt=urn:btih:xyz"},
			},
		},
	}
	if got := d.pickShareURL(obj3); !strings.Contains(got, "keepshare.org") {
		t.Fatalf("files keepshare = %q, want keepshare", got)
	}

	// ③ obj.URL 本身是分享
	obj4 := &model.DownloadObject{URL: "https://mypikpak.com/s/abc123"}
	if got := d.pickShareURL(obj4); got != obj4.URL {
		t.Fatalf("pickShareURL direct = %q, want %q", got, obj4.URL)
	}

	// 非分享 URL → 空
	obj5 := &model.DownloadObject{URL: "https://example.com/x.mp4"}
	if got := d.pickShareURL(obj5); got != "" {
		t.Fatalf("non-share URL should be empty, got %q", got)
	}
}

// TestSproxyHybrid_PollShortCircuit 验证 poll 对连续非 2xx 错误短路返回（避免 3h 空等）。
func TestSproxyHybrid_PollShortCircuit(t *testing.T) {
	mux := http.NewServeMux()
	// 任务详情恒 404
	mux.HandleFunc("GET /api/cloud/tasks/task-404", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{
		APIURL:    srv.URL + "/api/cloud/download",
		PollEvery: 10 * time.Millisecond,
		Timeout:   3 * time.Hour, // 若未短路会空等 3h
	})
	start := time.Now()
	err := d.poll("404")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("poll should return error on consecutive 404")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("poll did not short-circuit, took %v", elapsed)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("poll error should mention 404, got %v", err)
	}
}

// TestSproxyHybrid_SubmitHeaders 验证 submit 透传 headers。
func TestSproxyHybrid_SubmitHeaders(t *testing.T) {
	var gotReferer, gotUA string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		gotReferer = r.Header.Get("Referer")
		gotUA = r.Header.Get("User-Agent")
		writeJSONResp(w, map[string]any{"id": "task-1", "status": "running"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyHybridDownloader(config.SproxyHybridConfig{APIURL: srv.URL + "/api/cloud/download"})
	taskID, err := d.submit("https://mypikpak.com/s/abc", "out.mp4", map[string]string{
		"Referer":    "https://mypikpak.com/",
		"User-Agent": "test-agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if taskID != "task-1" {
		t.Fatalf("taskID = %q", taskID)
	}
	if gotReferer != "https://mypikpak.com/" {
		t.Fatalf("Referer not passed through: %q", gotReferer)
	}
	if gotUA != "test-agent" {
		t.Fatalf("User-Agent not passed through: %q", gotUA)
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
