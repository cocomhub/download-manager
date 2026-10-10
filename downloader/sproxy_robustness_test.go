// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/download"
)

// TestSproxyCloud_StatusURL 验证任务详情 URL 构造不受主机名/路径含 "download" 影响。
func TestSproxyCloud_StatusURL(t *testing.T) {
	t.Parallel()
	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{APIURL: "http://download.example.com/api/cloud/download"})
	if got, want := d.statusURL("t1"), "http://download.example.com/api/cloud/tasks/t1"; got != want {
		t.Fatalf("statusURL = %q, want %q", got, want)
	}
}

// TestSproxyCloud_ValidateSaveName 验证提交文件名校验。
func TestSproxyCloud_ValidateSaveName(t *testing.T) {
	t.Parallel()
	if err := validateSaveName("ok.mp4"); err != nil {
		t.Fatalf("ok name rejected: %v", err)
	}
	for _, bad := range []string{"a/b.mp4", `a\b.mp4`, "..", "."} {
		if err := validateSaveName(bad); err == nil {
			t.Errorf("name %q should be rejected", bad)
		}
	}
	if err := validateSaveName(""); err != nil {
		t.Errorf("empty name should be allowed (server derives): %v", err)
	}
}

// TestSproxyCloud_APIURLMissingFails 验证未配 api_url 时拒绝投递（不发给缺省地址）。
func TestSproxyCloud_APIURLMissingFails(t *testing.T) {
	t.Parallel()
	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{APIToken: "t"})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc"}
	err := d.Download(obj, nil)
	if err == nil {
		t.Fatal("Download should fail when api_url missing")
	}
	if !download.IsNoTry(err) {
		t.Fatalf("expected ErrNoTry (permanent), got %v", err)
	}
}

// TestSproxyCloud_PartialCredFails 验证三件套部分配置时拒绝（不静默回落）。
func TestSproxyCloud_PartialCredFails(t *testing.T) {
	t.Parallel()
	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:    "http://127.0.0.1:1/api/cloud/download",
		AccessKey: "ak-only", // 缺 secret/id → 半配置
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc"}
	err := d.Download(obj, nil)
	if err == nil {
		t.Fatal("Download should fail on partial credentials")
	}
	if !download.IsNoTry(err) {
		t.Fatalf("expected ErrNoTry, got %v", err)
	}
}

// TestSproxyCloud_CancelledStatusFails 验证服务端 cancelled 任务立即失败（不空转）。
func TestSproxyCloud_CancelledStatusFails(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-cx", "status": "running", "filename": "m.mp4"})
	})
	mux.HandleFunc("GET /api/cloud/tasks/task-cx", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-cx", "status": "cancelled", "filename": "m.mp4"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL: srv.URL + "/api/cloud/download", APIToken: "t", PollEvery: 10, Timeout: 10 * time.Second,
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc"}
	start := time.Now()
	if err := d.Download(obj, nil); err == nil {
		t.Fatal("Download should fail on cancelled status")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("cancelled should fail fast, took %v", time.Since(start))
	}
}

// TestSproxyCloud_ReusesExistingTask 验证幂等：Extra 已有 cloud_task_id 时不再 submit
// （避免 pullback 失败重试造成重复下载/转存）。
func TestSproxyCloud_ReusesExistingTask(t *testing.T) {
	t.Parallel()
	var submitCalls atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		submitCalls.Add(1)
		writeJSONResp(w, map[string]any{"id": "task-new", "status": "running"})
	})
	mux.HandleFunc("GET /api/cloud/tasks/task-pre", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-pre", "status": "completed", "filename": "m.mp4"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL: srv.URL + "/api/cloud/download", APIToken: "t", PollEvery: 10, CloudOnly: true,
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc"}
	obj.Extra = map[string]any{"cloud_task_id": "task-pre"}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if n := submitCalls.Load(); n != 0 {
		t.Fatalf("submit called %d times, want 0 (should reuse existing task)", n)
	}
	obj.RLock()
	fn, _ := obj.Extra["cloud_task_filename"].(string)
	obj.RUnlock()
	if fn != "m.mp4" {
		t.Fatalf("cloud_task_filename = %q, want m.mp4", fn)
	}
}

// TestSproxyCloud_PullbackAtomic 验证 pullback 原子落盘：失败不留 .partial、不留假完成文件。
func TestSproxyCloud_PullbackAtomic(t *testing.T) {
	t.Parallel()
	ak := "ak-atomic"
	sk := "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
	skid := "skey-atomic001"
	mux := http.NewServeMux()
	mockCredList(mux, ak, skid)
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-at", "status": "completed", "filename": "m.mp4"})
	})
	mux.HandleFunc("GET /api/cloud/tasks/task-at", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-at", "status": "completed", "filename": "m.mp4"})
	})
	mux.HandleFunc("HEAD /api/files/stat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-File-Size", "4")
		w.WriteHeader(http.StatusOK)
	})
	// 分块下载恒 500 → pullback 失败
	mux.HandleFunc("GET /download/chunk", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL: srv.URL + "/api/cloud/download", AccessKey: ak, AccessKeySecret: sk, AccessKeyID: skid,
		PollEvery: 10,
	})
	dir := t.TempDir()
	savePath := filepath.Join(dir, "out.mp4")
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc", SavePath: savePath}
	if err := d.Download(obj, nil); err == nil {
		t.Fatal("Download should fail when pullback chunk fails")
	}
	if _, err := os.Stat(savePath); !os.IsNotExist(err) {
		t.Fatalf("savePath must not exist after failed pullback (stat err=%v)", err)
	}
	if _, err := os.Stat(savePath + ".partial"); !os.IsNotExist(err) {
		t.Fatalf(".partial must be cleaned up after failed pullback (stat err=%v)", err)
	}
}

// TestSproxyCloud_TerminalFailureClearsTaskID 验证 sproxy 侧任务终态失败后清除
// cloud_task_id，使重试重新提交（否则重试永远复用已失效任务直到永久失败）。
func TestSproxyCloud_TerminalFailureClearsTaskID(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/cloud/tasks/task-dead", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-dead", "status": "failed", "filename": "m.mp4"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL: srv.URL + "/api/cloud/download", APIToken: "t", PollEvery: 10, CloudOnly: true,
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc"}
	obj.Extra = map[string]any{"cloud_task_id": "task-dead"}
	if err := d.Download(obj, nil); err == nil {
		t.Fatal("终态 failed 应返回错误")
	}
	obj.RLock()
	_, still := obj.Extra["cloud_task_id"]
	obj.RUnlock()
	if still {
		t.Fatal("终态失败后应清除 cloud_task_id（否则重试永远复用死任务）")
	}
}
