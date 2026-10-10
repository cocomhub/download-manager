// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// TestSproxyCloud_TaskGoneClearsTaskID 验证 sproxy 侧任务 404（已清理/重启）后清除
// cloud_task_id，使重试重新提交（否则重试持续打同一 404 id 直到永久失败）。
func TestSproxyCloud_TaskGoneClearsTaskID(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/cloud/tasks/task-404", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "task not found", http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL: srv.URL + "/api/cloud/download", APIToken: "t", PollEvery: 10, CloudOnly: true,
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc"}
	obj.Extra = map[string]any{"cloud_task_id": "task-404"}
	if err := d.Download(obj, nil); err == nil {
		t.Fatal("404 应返回错误")
	}
	obj.RLock()
	_, still := obj.Extra["cloud_task_id"]
	obj.RUnlock()
	if still {
		t.Fatal("任务 404 后应清除 cloud_task_id（否则重试永远复用死任务）")
	}
}

// TestSproxyCloud_BearerRequiresCloudOnly 验证 Bearer 模式（无 SproxySig）配
// cloud_only=false 时提交前即拒绝（此前会白跑一次服务端下载/转存后才失败）。
func TestSproxyCloud_BearerRequiresCloudOnly(t *testing.T) {
	t.Parallel()
	submitCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		submitCalls++
		writeJSONResp(w, map[string]any{"id": "t", "status": "running"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL: srv.URL + "/api/cloud/download", APIToken: "bearer", PollEvery: 10,
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc", SavePath: filepath.Join(t.TempDir(), "o.bin")}
	err := d.Download(obj, nil)
	if err == nil {
		t.Fatal("Bearer + cloud_only=false 应拒绝")
	}
	if !errors.Is(err, download.ErrNoTry) {
		t.Fatalf("应为 ErrNoTry（不重试）, got %v", err)
	}
	if submitCalls != 0 {
		t.Fatalf("应提交前拒绝, 但发生了 %d 次 submit", submitCalls)
	}
}

// TestSproxyCloud_PullbackFailureKeepsProgressBelow100 验证本地下载失败时不置 100%
// （否则 UI 显示「满进度 + failed」）。
func TestSproxyCloud_PullbackFailureKeepsProgressBelow100(t *testing.T) {
	t.Parallel()
	const ak, skid = "ak-pb", "skey-pb00000001"
	sk := strings.Repeat("9", 64)
	mux := http.NewServeMux()
	mockCredList(mux, ak, skid)
	mux.HandleFunc("GET /api/cloud/tasks/task-pb", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-pb", "status": "completed", "filename": "m.mp4"})
	})
	mux.HandleFunc("GET /download/chunk", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: sk,
		AccessKeyID:     skid,
		PollEvery:       10,
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc", SavePath: filepath.Join(t.TempDir(), "o.bin")}
	obj.Extra = map[string]any{"cloud_task_id": "task-pb"}
	if err := d.Download(obj, nil); err == nil {
		t.Fatal("本地下载失败应返回错误")
	}
	if p := obj.GetProgress(); p >= 100 {
		t.Fatalf("失败时不应置 100%%, got %d", p)
	}
}

// TestSproxyCloud_SigTaskGoneClearsTaskID 验证 SproxySig 路径下 404（sproxy 客户端映射为
// ErrNotFound，而非本地 errCloudTaskGone）同样清除失效坐标。
func TestSproxyCloud_SigTaskGoneClearsTaskID(t *testing.T) {
	t.Parallel()
	const ak, skid = "ak-404sig", "skey-404sig0001"
	sk := strings.Repeat("5", 64)
	mux := http.NewServeMux()
	mockCredList(mux, ak, skid)
	mux.HandleFunc("GET /api/cloud/tasks/task-404sig", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "task not found", http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: sk,
		AccessKeyID:     skid,
		PollEvery:       10,
		CloudOnly:       true,
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc"}
	obj.Extra = map[string]any{"cloud_task_id": "task-404sig"}
	if err := d.Download(obj, nil); err == nil {
		t.Fatal("sig 路径 404 应返回错误")
	}
	obj.RLock()
	_, still := obj.Extra["cloud_task_id"]
	obj.RUnlock()
	if still {
		t.Fatal("sig 路径任务 404 后应清除 cloud_task_id")
	}
}

// TestValidateSaveName_AlignsWithSproxySafe 验证文件名校验与 sproxy cloudfilename.Safe 完全一致
// （此前 dm 自实现比服务端宽松 → 含 ? : < > | " * 等会被服务端 400）。
func TestValidateSaveName_AlignsWithSproxySafe(t *testing.T) {
	t.Parallel()
	valid := []string{"", "movie.mp4", "中文 名.mp4", "a_b-c.d.mp4"}
	for _, n := range valid {
		if err := validateSaveName(n); err != nil {
			t.Errorf("合法名 %q 应通过: %v", n, err)
		}
	}
	invalid := []string{"a?b.mp4", "a:b.mp4", "a<b.mp4", "a|b.mp4", "a*b.mp4", "a\tb.mp4", "a/b.mp4", "a" + string(rune(92)) + "b.mp4", ".hidden.", "CON", "a.mp4 "}
	for _, n := range invalid {
		if err := validateSaveName(n); err == nil {
			t.Errorf("非法名 %q 应被拒绝（Safe 会改写它）", n)
		}
	}
}

// TestSproxyCloud_NonHTTPURLRejected 验证非 http(s) URL 早退为永久失败（服务端只收 http/https）。
func TestSproxyCloud_NonHTTPURLRejected(t *testing.T) {
	t.Parallel()
	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL: "http://127.0.0.1:1/api/cloud/download", APIToken: "t", CloudOnly: true,
	})
	for _, u := range []string{"magnet:?xt=urn:btih:abc", "bt://x", "ftp://host/f"} {
		obj := &model.DownloadObject{URL: u}
		err := d.Download(obj, nil)
		if err == nil || !errors.Is(err, download.ErrNoTry) {
			t.Errorf("%q 应为 ErrNoTry, got %v", u, err)
		}
	}
}

// TestSproxyCloud_SubmitDeclaresProducePolicy 验证提交体显式声明 save/download_local
// （不依赖服务端默认：save=false 会在完成后删除 cloud 桶副本，使本地拉回失败）。
func TestSproxyCloud_SubmitDeclaresProducePolicy(t *testing.T) {
	t.Parallel()
	var got map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		writeJSONResp(w, map[string]any{"id": "t1", "status": "running"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL: srv.URL + "/api/cloud/download", APIToken: "t", CloudOnly: true,
	})
	if _, err := d.submit(t.Context(), "https://example.com/a.mp4", "", nil); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if got["save"] != true {
		t.Fatalf("save 应显式为 true, got %v", got["save"])
	}
	if got["download_local"] != false { // CloudOnly=true → 不拉回本地
		t.Fatalf("download_local 应显式为 false, got %v", got["download_local"])
	}
}

// TestSproxyCloud_DamagedIntegrityFails 验证 integrity_status=damaged 不当作成功。
func TestSproxyCloud_DamagedIntegrityFails(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/cloud/tasks/task-dmg", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-dmg", "status": "completed", "filename": "m.mp4", "integrity_status": "damaged"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL: srv.URL + "/api/cloud/download", APIToken: "t", PollEvery: 10, CloudOnly: true,
	})
	obj := &model.DownloadObject{URL: "https://example.com/a.mp4"}
	obj.Extra = map[string]any{"cloud_task_id": "task-dmg"}
	err := d.Download(obj, nil)
	if err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("damaged 应失败, got %v", err)
	}
	obj.RLock()
	_, still := obj.Extra["cloud_task_id"]
	obj.RUnlock()
	if still {
		t.Fatal("damaged 后应清坐标以便换源重试")
	}
}
