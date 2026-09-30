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
	prodName := "download" // Gopeed 落盘名（直链 URL 推断），与 SavePath 不同名
	// 产物在 SavePath 所在目录（Gopeed 落盘语义：与 jpg/preview 同目录）
	if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
		t.Fatal(err)
	}
	prodPath := filepath.Join(filepath.Dir(savePath), prodName)
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
	// 磁力 URL：验证 createTask + waitAndMove 整条路径（Gopeed 对磁力/直链均走 http 任务）。
	obj := &model.DownloadObject{URL: "magnet:?xt=urn:btih:abcdef1234567890abcdef1234567890", SavePath: savePath}
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
	err := d.Download(&model.DownloadObject{URL: "magnet:?xt=urn:btih:abcdef1234567890abcdef1234567890", SavePath: filepath.Join(dir, "x")}, nil)
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
	err := d.Download(&model.DownloadObject{URL: "magnet:?xt=urn:btih:abcdef1234567890abcdef1234567890", SavePath: filepath.Join(dir, "x")}, nil)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v, want timed out", err)
	}
}

// TestGopeedDownload_UnsupportedDetailPage 验证：普通 http 详情页 URL（非磁力、非 PikPak）
// 在入口被拦截，返回 ErrUnsupportedURL，不创建 Gopeed http 任务（避免下 HTML/页面）。
func TestGopeedDownload_UnsupportedDetailPage(t *testing.T) {
	// 无 /api/v1/tasks 处理器 —— 若误建任务会 404/超时，但正确行为应根本不触达
	d := &GopeedDownloader{
		rpcURL:       "http://127.0.0.1:1", // 不可达，防止意外网络请求
		pollInterval: 10 * time.Millisecond,
		timeout:      500 * time.Millisecond,
		httpClient:   &http.Client{Timeout: 500 * time.Millisecond},
	}
	obj := &model.DownloadObject{URL: "https://njavtv.com/ja/sample-123", SavePath: filepath.Join(t.TempDir(), "x.mp4")}
	err := d.Download(obj, nil)
	if err == nil {
		t.Fatal("expected ErrUnsupportedURL for plain detail page URL, got nil")
	}
	if !errors.Is(err, ErrUnsupportedURL) {
		t.Fatalf("err = %v, want ErrUnsupportedURL", err)
	}
	// ensure 未尝试创建任务：直接在 rpc 层等价 —— 若创建，会请求失败，但入口拦截应返回 ErrUnsupportedURL
	// （上述 err 已覆盖该行为）
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

// gopeedPikPakServer 构造 PikPak 场景的 mock：
//   - /keepshare 返回 302 → /s/<share_id>
//   - POST /api/v1/resolve 返回扩展解析结果（文件列表含直链）
//   - POST /api/v1/tasks + GET /api/v1/tasks/<id> 走普通 http 下载轮询
func gopeedPikPakServer(t *testing.T, files []map[string]any) string {
	t.Helper()
	var dlCalls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/keepshare":
			http.Redirect(w, r, "/s/voyza0000", http.StatusFound)
		case r.URL.Path == "/s/voyza0000":
			// keepshare 302 跟随后的最终页（真实是 mypikpak 分享页），返回 200 即可
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/resolve":
			res := map[string]any{"res": map[string]any{"files": files}}
			writeGopeedResponse(w, res)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
			writeGopeedResponse(w, "dl-1")
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks/dl-1":
			n := dlCalls.Add(1)
			task := gopeedTask{ID: "dl-1", Status: "done", Meta: gopeedMeta{Res: gopeedRes{Files: []gopeedFile{{Name: "target.mp4"}}}}}
			if n == 1 {
				task.Status = "running"
			}
			writeGopeedResponse(w, task)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestGopeedDownload_PikPakKeepShare 验证 keepshare 磁力镜像 URL 走 PikPak 分支：
// keepshare 302 → resolve 免登录直链 → 按 dn 匹配目标 → 直链走 http 下载移动到 SavePath。
func TestGopeedDownload_PikPakKeepShare(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "out", "sample-123.mp4")
	// 产物在 SavePath 所在目录（Gopeed 落盘语义）
	if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
		t.Fatal(err)
	}
	prodPath := filepath.Join(filepath.Dir(savePath), "target.mp4")
	if err := os.WriteFile(prodPath, []byte("full-movie"), 0644); err != nil {
		t.Fatal(err)
	}

	files := []map[string]any{
		{"name": "promo.png", "size": 1000, "req": map[string]any{"url": "http://x/promo.png"}},
		{"name": "SAMPLE-123-uncensored-full.mp4", "size": 1024,
			"req": map[string]any{"url": "https://dl.mypikpak.com/download/?fid=abc", "extra": map[string]any{"header": map[string]any{"Referer": "https://mypikpak.com/", "User-Agent": "UA"}}}},
	}
	rpc := gopeedPikPakServer(t, files)
	d := gopeedTestDownloader(rpc, dir)

	// keepshare URL 指向 mock server（避免真实网络请求 mypikpak.com）
	keepshareURL := rpc + "/keepshare?dn=SAMPLE-123-uncensored-HD"
	obj := &model.DownloadObject{URL: keepshareURL, SavePath: savePath}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download() error: %v", err)
	}
	if _, err := os.Stat(savePath); err != nil {
		t.Errorf("产物未到 SavePath %s: %v", savePath, err)
	}
}

// TestGopeedDownload_PikPakNoMatch 验证 resolve 出文件但无匹配 → 明确错误。
func TestGopeedDownload_PikPakNoMatch(t *testing.T) {
	dir := t.TempDir()
	files := []map[string]any{
		{"name": "promo.png", "size": 1000, "req": map[string]any{"url": "http://x/promo.png"}},
		{"name": "back.jpg", "size": 2000, "req": map[string]any{"url": "http://x/back.jpg"}},
	}
	rpc := gopeedPikPakServer(t, files)
	d := gopeedTestDownloader(rpc, dir)
	// 同样指向 mock server（避免真实网络请求 keepshare.org）
	keepshareURL := rpc + "/keepshare?dn=SAMPLE-999"
	err := d.Download(&model.DownloadObject{URL: keepshareURL, SavePath: filepath.Join(dir, "x.mp4")}, nil)
	if err == nil {
		t.Fatal("expected error when no matching pikpak file, got nil")
	}
	if !strings.Contains(err.Error(), "no matching file") {
		t.Errorf("err = %v, want mention no matching file", err)
	}
}

// TestGopeedDownload_PikPakDirectShare 验证直接 mypikpak.com/s/<id> 分享链接（无 keepshare）也走 PikPak 分支。
func TestGopeedDownload_PikPakDirectShare(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "out", "full.mp4")
	if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
		t.Fatal(err)
	}
	prodPath := filepath.Join(filepath.Dir(savePath), "target.mp4")
	if err := os.WriteFile(prodPath, []byte("movie"), 0644); err != nil {
		t.Fatal(err)
	}
	files := []map[string]any{
		{"name": "SAMPLE-123-uncensored-HD.mp4", "size": 100, "req": map[string]any{"url": "https://dl.mypikpak.com/download/?fid=zzz"}},
	}
	rpc := gopeedPikPakServer(t, files)
	d := gopeedTestDownloader(rpc, dir)
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abcdef0123456789abcdef0123456789", SavePath: savePath}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download() error: %v", err)
	}
	if _, err := os.Stat(savePath); err != nil {
		t.Errorf("产物未到 SavePath %s: %v", savePath, err)
	}
}

// TestPickPikPakTarget_SingleNonVideoFile 验证「解析出唯一文件但无视频扩展名」也接受（磁力单文件兜底）。
func TestPickPikPakTarget_SingleNonVideoFile(t *testing.T) {
	d := &GopeedDownloader{}
	files := []pikpakFile{
		{Name: "movie.2026", Size: 1024, DownloadURL: "http://dl/f1"},
	}
	got := d.pickPikPakTarget(files, "https://keepshare.org/abc/magnet:?xt=urn:btih:deadbeef&dn=MOVIE-001")
	if got == nil {
		t.Fatal("pickPikPakTarget single file should accept even without video ext")
	}
	if got.DownloadURL != "http://dl/f1" {
		t.Errorf("got %v, want f1", got.DownloadURL)
	}
}

// TestPickPikPakTarget_MultiNonVideoNil 验证「多文件且都无视频扩展」仍返回 nil（避免误下缩略图）。
func TestPickPikPakTarget_MultiNonVideoNil(t *testing.T) {
	d := &GopeedDownloader{}
	files := []pikpakFile{
		{Name: "cover.jpg", Size: 100, DownloadURL: "http://dl/cover"},
		{Name: "preview.ts.bak", Size: 50, DownloadURL: "http://dl/pv"},
	}
	got := d.pickPikPakTarget(files, "https://keepshare.org/abc/magnet:?xt=urn:btih:deadbeef&dn=X")
	if got != nil {
		t.Errorf("multi non-video should return nil, got %+v", got)
	}
}

// TestPickPikPakTarget_VideoExtPick 验证多文件时优先视频扩展（不再只认 mp4）。
func TestPickPikPakTarget_VideoExtPick(t *testing.T) {
	d := &GopeedDownloader{}
	files := []pikpakFile{
		{Name: "cover.jpg", Size: 100, DownloadURL: "http://dl/cover"},
		{Name: "MOVIE-001.full.mkv", Size: 2048, DownloadURL: "http://dl/mkv"},
		{Name: "trailer.mp4", Size: 500, DownloadURL: "http://dl/tr"},
	}
	got := d.pickPikPakTarget(files, "https://keepshare.org/abc/magnet:?xt=urn:btih:deadbeef&dn=MOVIE-001")
	if got == nil || got.DownloadURL != "http://dl/mkv" {
		t.Fatalf("want largest video (mkv), got %+v", got)
	}
}
