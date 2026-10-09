// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
)

// TestSproxyCloud_CancelInterruptsPoll 验证 Cancel(url) 能中断在途轮询
// （对抗性评审 P1-1：取消后不得让 worker 槽位被占满整个 timeout）。
func TestSproxyCloud_CancelInterruptsPoll(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
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
	})
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc"}

	done := make(chan error, 1)
	go func() { done <- d.Download(obj, nil) }()

	// 等 submit + 至少一次轮询发生
	time.Sleep(300 * time.Millisecond)
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

// TestSproxyCloud_SetContextCancelStopsPoll 验证注入 ctx 取消（停机 drain 场景）
// 也能中断轮询。
func TestSproxyCloud_SetContextCancelStopsPoll(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
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
	})
	ctx, cancel := context.WithCancel(context.Background())
	d.SetContext(ctx)
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc"}

	done := make(chan error, 1)
	go func() { done <- d.Download(obj, nil) }()
	time.Sleep(300 * time.Millisecond)
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
