// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
)

// TestSproxyCloud_CancelInterruptsPoll 验证 Cancel(url) 能中断在途轮询
// （对抗性评审 P1-1：取消后不得让 worker 槽位被占满整个 timeout）。
func TestSproxyCloud_CancelInterruptsPoll(t *testing.T) {
	t.Parallel()
	submitted := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-submitted:
		default:
			close(submitted)
		}
		writeJSONResp(w, map[string]any{"id": "task-cancel", "status": "running", "filename": "m.mp4"})
	})
	// 任务恒 running（永不完成）→ 只有 Cancel 能终止 Download
	mux.HandleFunc("GET /api/cloud/tasks/task-cancel", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-cancel", "status": "running", "filename": "m.mp4"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:    srv.URL + "/api/cloud/download",
		APIToken:  "bearer",
		PollEvery: 500 * time.Millisecond,
		Timeout:   30 * time.Second, // 远大于测试窗口：只有 Cancel 能让它提前返回
		CloudOnly: true,             // Bearer 模式不支持本地下载
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc"}

	done := make(chan error, 1)
	go func() { done <- d.Download(obj, nil) }()

	// 等 submit 真正发生（之后 registerCancel 已登记），避免用 sleep 造成 flake
	select {
	case <-submitted:
	case <-time.After(2 * time.Second):
		t.Fatal("submit did not happen")
	}
	if err := d.Cancel(obj.URL); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Download should return error after Cancel")
		}
	case <-time.After(1200 * time.Millisecond):
		t.Fatal("Cancel did not interrupt in-flight poll (P1-1 regression)")
	}
}

// TestSproxyCloud_PerURLCtxIsolation 验证按 URL 注入的 ctx 互不覆盖：
// 兄弟下载（共享实例）取消自己的 ctx 不得影响本对象的在途下载。
func TestSproxyCloud_PerURLCtxIsolation(t *testing.T) {
	t.Parallel()
	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{APIURL: "http://127.0.0.1:1/x", APIToken: "t"})
	ctxA, cancelA := context.WithCancel(t.Context())
	ctxB, cancelB := context.WithCancel(t.Context())
	defer cancelB()

	// 模拟：兄弟下载已把共享字段覆盖为 A 的 ctx，而 B 按 URL 注入自己的 ctx
	d.SetContext(ctxA)
	d.SetContextFor("http://b", ctxB)
	cancelA() // A 的下载结束 → 其 dlCancel 触发

	if err := d.reqCtxFor("http://b").Err(); err != nil {
		t.Fatalf("B 应使用按 URL 注入的 ctx，不应被 A 的取消影响: %v", err)
	}
	// 未按 URL 注入的 URL 回落进程级 ctx（已取消）
	if err := d.reqCtxFor("http://c").Err(); err == nil {
		t.Fatal("未注入 URL 应回落进程级 ctx（此时已取消）")
	}
	// 清理后回落
	d.clearContextFor("http://b")
	if err := d.reqCtxFor("http://b").Err(); err == nil {
		t.Fatal("clearContextFor 后应回落进程级 ctx")
	}
}

// TestSproxyCloud_SetContextCancelStopsPoll 验证注入 ctx 取消（停机 drain 场景）
// 也能中断轮询。
func TestSproxyCloud_SetContextCancelStopsPoll(t *testing.T) {
	t.Parallel()
	submitted := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-submitted:
		default:
			close(submitted)
		}
		writeJSONResp(w, map[string]any{"id": "task-ctx", "status": "running", "filename": "m.mp4"})
	})
	mux.HandleFunc("GET /api/cloud/tasks/task-ctx", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-ctx", "status": "running", "filename": "m.mp4"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:    srv.URL + "/api/cloud/download",
		APIToken:  "bearer",
		PollEvery: 500 * time.Millisecond,
		Timeout:   30 * time.Second,
		CloudOnly: true, // Bearer 模式不支持本地下载
	})
	ctx, cancel := context.WithCancel(t.Context())
	d.SetContext(ctx)
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc"}

	done := make(chan error, 1)
	go func() { done <- d.Download(obj, nil) }()
	select {
	case <-submitted:
	case <-time.After(2 * time.Second):
		t.Fatal("submit did not happen")
	}
	cancel() // 模拟停机 drain

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Download should return error after SetContext ctx cancel")
		}
	case <-time.After(1200 * time.Millisecond):
		t.Fatal("ctx cancel did not interrupt in-flight poll")
	}
}

// TestSproxyCloud_CancelCancelsRemoteTask 验证 Cancel 会连带取消 sproxy 侧任务
// （否则本地已取消，但服务端仍继续下载/转存，占用服务端资源与配额）。
func TestSproxyCloud_CancelCancelsRemoteTask(t *testing.T) {
	t.Parallel()
	const ak, skid = "ak-cancel", "skey-cancel0001"
	sk := strings.Repeat("7", 64)
	remote := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mockCredList(mux, ak, skid)
	mux.HandleFunc("POST /api/cloud/tasks/task-rc/cancel", func(w http.ResponseWriter, r *http.Request) {
		select {
		case remote <- struct{}{}:
		default:
		}
		writeJSONResp(w, map[string]any{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: sk,
		AccessKeyID:     skid,
	})
	d.registerTaskID("https://mypikpak.com/s/abc", "task-rc")
	if err := d.Cancel("https://mypikpak.com/s/abc"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	select {
	case <-remote:
	case <-time.After(3 * time.Second):
		t.Fatal("Cancel 未连带取消 sproxy 侧任务")
	}
}

// TestSproxyCloud_TryReverifyConcurrent 并发懒重验不产生数据竞争（配合 -race 守门）。
func TestSproxyCloud_TryReverifyConcurrent(t *testing.T) {
	t.Parallel()
	const ak, skid = "ak-rv", "skey-rv00000001"
	sk := strings.Repeat("8", 64)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/credentials/"+ak+"/sk", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:          srv.URL + "/api/cloud/download",
		AccessKey:       ak,
		AccessKeySecret: sk,
		AccessKeyID:     skid,
	})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_ = d.tryReverify()
		})
	}
	wg.Wait()
	if d.verified.Load() {
		t.Fatal("凭据列表失败时 verified 应为 false")
	}
}
