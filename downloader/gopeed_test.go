// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"encoding/json"
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
)

// gopeedTestServer 模拟 Gopeed REST API：POST /api/v1/tasks 返回固定任务 id，
// GET /api/v1/tasks/task-1 由 pollHook 决定每次轮询返回的任务对象。
// pollHook 为空表示任务一直 running（用于超时测试）。
func gopeedTestServer(t *testing.T, pollHook func(call int) *gopeedTask) string {
	t.Helper()
	// 模拟 Gopeed API：POST 创建任务返回 task id，GET 查询任务状态。
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
			writeGopeedResponse(w, "task-1")
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks/task-1":
			n := calls.Add(1)
			var task gopeedTask
			if pollHook != nil {
				task = *pollHook(int(n))
			} else {
				task = gopeedTask{ID: "task-1", Status: "running"}
			}
			writeGopeedResponse(w, task)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func writeGopeedResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	payload, _ := json.Marshal(map[string]any{"code": 0, "message": "", "data": data})
	w.Write(payload)
}

// gopeedTestDownloader 构造带短轮询间隔/短超时/短 HTTP 超时的测试下载器。
func gopeedTestDownloader(rpcURL, downloadDir string) *GopeedDownloader {
	return &GopeedDownloader{
		rpcURL:       rpcURL,
		downloadDir:  downloadDir,
		pollInterval: 10 * time.Millisecond,
		timeout:      5 * time.Second,
		httpClient:   &http.Client{Timeout: 2 * time.Second},
	}
}

func TestNew_Gopeed(t *testing.T) {
	d := New(config.Downloader{Type: "gopeed"})
	// New(type=gopeed) 走 NewGopeedDownloader，其 rpcURL/poll/timeout 依赖配置默认填充（ValidateAndClamp）。
	// 此处只验证注册与 Name，不依赖网络。
	if d == nil {
		t.Fatal("New(type=gopeed) returned nil")
	}
	if got := d.Name(); got != "gopeed" {
		t.Errorf("Name() = %q, want gopeed", got)
	}
}

func TestNewGopeedDownloader_Defaults(t *testing.T) {
	d := NewGopeedDownloader(config.Downloader{})
	if got := d.Name(); got != "gopeed" {
		t.Errorf("Name() = %q, want gopeed", got)
	}
	if d.rpcURL != "http://127.0.0.1:9999" {
		t.Errorf("rpcURL = %q, want default http://127.0.0.1:9999", d.rpcURL)
	}
}

// TestGopeedDownload_Success 验证：轮询到 done 后，产物从 DownloadDir 移动到 obj.SavePath。
func TestGopeedDownload_Success(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "out", "movie.mp4")
	prodName := "movie.mp4"
	prodPath := filepath.Join(dir, prodName)
	if err := os.WriteFile(prodPath, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	rpc := gopeedTestServer(t, func(call int) *gopeedTask {
		if call == 1 {
			return &gopeedTask{ID: "task-1", Status: "running"}
		}
		return &gopeedTask{
			ID:     "task-1",
			Status: "done",
			Meta:   gopeedMeta{Res: gopeedRes{Files: []gopeedFile{{Name: prodName}}}},
		}
	})

	d := gopeedTestDownloader(rpc, dir)
	obj := &model.DownloadObject{URL: "http://example.com/movie.mp4", SavePath: savePath}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download() returned error: %v", err)
	}

	if _, err := os.Stat(savePath); err != nil {
		t.Errorf("产物未移动到 SavePath %s: %v", savePath, err)
	}
	if _, err := os.Stat(prodPath); !os.IsNotExist(err) {
		t.Errorf("源产物应被移走，但仍存在 %s（err=%v）", prodPath, err)
	}
}

// TestGopeedDownload_TaskError 验证 status=error 返回错误。
func TestGopeedDownload_TaskError(t *testing.T) {
	dir := t.TempDir()
	rpc := gopeedTestServer(t, func(call int) *gopeedTask {
		return &gopeedTask{ID: "task-1", Status: "error"}
	})
	d := gopeedTestDownloader(rpc, dir)
	err := d.Download(&model.DownloadObject{URL: "http://example.com/f", SavePath: filepath.Join(dir, "x")}, nil)
	if err == nil {
		t.Fatal("expected error for status=error task, got nil")
	}
	if !strings.Contains(err.Error(), "status=error") {
		t.Errorf("err = %v, want mention status=error", err)
	}
}

// TestGopeedDownload_Timeout 验证一直 running 时在 timeout 后返回错误。
func TestGopeedDownload_Timeout(t *testing.T) {
	dir := t.TempDir()
	rpc := gopeedTestServer(t, nil) // 一直 running
	d := gopeedTestDownloader(rpc, dir)
	d.timeout = 80 * time.Millisecond
	err := d.Download(&model.DownloadObject{URL: "http://example.com/f", SavePath: filepath.Join(dir, "x")}, nil)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v, want timed out", err)
	}
}

func TestResolveProtocol(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"magnet:?xt=urn:btih:abc", "magnet"},
		{"magnetic://abc", "magnet"},
		{"bt://abc", "magnet"},
		{"ed2k://abc", "magnet"},
		{"http://example.com/f", "http"},
		{"https://example.com/f", "http"},
		{"ftp://example.com/f", "default"},
		{"some-unknown", "default"},
	}
	for _, tt := range tests {
		if got := resolveProtocol(tt.url); got != tt.want {
			t.Errorf("resolveProtocol(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}
