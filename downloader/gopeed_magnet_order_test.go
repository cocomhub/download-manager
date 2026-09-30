// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cocomhub/download-manager/model"
)

// Test_parseSizeBytes 验证人类可读大小解析为字节数。
func Test_parseSizeBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want int64
	}{
		{"", -1},
		{"abc", -1},
		{"4.65GB", 4992899481},
		{"500MB", 524288000},
		{"100KB", 102400},
		{"2GB", 2147483648},
	}
	for _, tt := range tests {
		if got := parseSizeBytes(tt.in); got != tt.want {
			t.Errorf("parseSizeBytes(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// Test_collectPikPakCandidates_SortBySizeDesc 验证 magnet_list 候选按 size 降序排列：
// keepshare 镜像优先（无 keepshare 用纯 magnet），size 解析失败（空）垫底保持原始顺序。
func Test_collectPikPakCandidates_SortBySizeDesc(t *testing.T) {
	t.Parallel()
	obj := &model.DownloadObject{Extra: map[string]any{
		"magnet_list": []map[string]string{
			{"magnet": "m1", "name": "mid", "keepshare": "https://mypikpak.com/s/mid", "size": "500MB"},
			{"magnet": "m2", "name": "big", "keepshare": "https://mypikpak.com/s/big", "size": "2GB"},
			{"magnet": "m3", "name": "small", "keepshare": "https://mypikpak.com/s/small", "size": "100KB"},
			{"magnet": "m4", "name": "nosize", "keepshare": "https://mypikpak.com/s/nosize", "size": ""},
			{"magnet": "m5", "name": "nokeepshare", "size": "300MB"}, // 无 keepshare，用纯磁
		},
	}}
	d := &GopeedDownloader{}
	got := d.collectPikPakCandidates(obj)

	wantURLs := []string{
		"https://mypikpak.com/s/big",    // 2GB
		"https://mypikpak.com/s/mid",    // 500MB
		"m5",                            // 300MB 纯磁
		"https://mypikpak.com/s/small",  // 100KB
		"https://mypikpak.com/s/nosize", // 无 size 垫底
	}
	if len(got) != len(wantURLs) {
		t.Fatalf("len = %d, want %d: %+v", len(got), len(wantURLs), got)
	}
	for i := range wantURLs {
		if got[i].url != wantURLs[i] {
			t.Errorf("candidate[%d].url = %q, want %q", i, got[i].url, wantURLs[i])
		}
	}
	if got[2].name != "nokeepshare" {
		t.Errorf("candidate[2].name = %q, want nokeepshare", got[2].name)
	}
}

// pikpakSIDFromBody 从 resolve 或 create task 请求 body 提取分享/直链标识（<sid> 或解析出的目标）。
// resolve body: {"url":"https://mypikpak.com/s/<sid>"...}；create body: {"url":"https://dl/<sid>.mp4"...}。
// 命中 "mypikpak.com/s/" 取 <sid>；命中 "dl/" 取直链中的 <sid>。
func pikpakSIDFromBody(r *http.Request) string {
	body := make([]byte, r.ContentLength)
	_, _ = r.Body.Read(body)
	bs := string(body)
	if i := strings.Index(bs, "mypikpak.com/s/"); i >= 0 {
		return sidAfter(bs, i+len("mypikpak.com/s/"))
	}
	if i := strings.Index(bs, "https://dl/"); i >= 0 {
		return sidAfter(bs, i+len("https://dl/"))
	}
	return ""
}

func sidAfter(s string, start int) string {
	rest := s[start:]
	out := ""
	for j := 0; j < len(rest); j++ {
		if rest[j] == '"' || rest[j] == '\\' || rest[j] == '/' || rest[j] == '.' {
			break
		}
		out += string(rest[j])
	}
	return out
}

// magnetSrv 构造 magnet_list 多候选场景 mock。
// 每个候选 = mypikpak.com/s/<sid> URL；resolve 返回一个 MV.mp4（直链 https://dl/<sid>.mp4）。
// success[sid] 决定该候选 http 下载终态：true=done（落盘 target.mp4 产物），false=error。
// getOrder() 返回候选尝试顺序（share sid）。
func magnetSrv(t *testing.T, success map[string]bool) (rpc string, getOrder func() []string) {
	t.Helper()
	var mu sync.Mutex
	var order []string
	var cur string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/resolve":
			sid := pikpakSIDFromBody(r)
			mu.Lock()
			order = append(order, sid)
			cur = sid
			mu.Unlock()
			writeGopeedResponse(w, map[string]any{"res": map[string]any{"files": []map[string]any{
				{"name": "MV.mp4", "size": 100, "req": map[string]any{"url": "https://dl/" + sid + ".mp4"}},
			}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
			writeGopeedResponse(w, "dl-1")
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks/dl-1":
			task := gopeedTask{ID: "dl-1", Status: "done", Meta: gopeedMeta{Res: gopeedRes{Files: []gopeedFile{{Name: "target.mp4"}}}}}
			if !success[cur] {
				task.Status = "error"
			}
			writeGopeedResponse(w, task)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string{}, order...)
	}
}

// TestGopeedDownload_MagnetList_SecondCandidateSucceeds
// magnet_list 两条：big（2GB，下载失败）→ small（500MB，下载成功）。
// 期望：按 size 降序先尝试 big（download error）再 small（done）；成功后移动 small 产物，返回 nil。
func TestGopeedDownload_MagnetList_SecondCandidateSucceeds(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "out", "movie.mp4")
	if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
		t.Fatal(err)
	}
	prodPath := filepath.Join(filepath.Dir(savePath), "target.mp4")
	if err := os.WriteFile(prodPath, []byte("cand-small"), 0644); err != nil {
		t.Fatal(err)
	}

	// small 候选下载成功；big 下载失败（error）
	rpc, order := magnetSrv(t, map[string]bool{"small": true})
	d := gopeedTestDownloader(rpc, dir)

	obj := &model.DownloadObject{
		SavePath: savePath,
		Extra: map[string]any{
			"magnet_list": []map[string]string{
				{"magnet": "m1", "name": "big", "keepshare": "https://mypikpak.com/s/big", "size": "2GB"},
				{"magnet": "m2", "name": "small", "keepshare": "https://mypikpak.com/s/small", "size": "500MB"},
			},
		},
	}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download() error: %v", err)
	}
	got := order()
	if len(got) != 2 || got[0] != "big" || got[1] != "small" {
		t.Fatalf("resolve order = %v, want [big small]", got)
	}
	data, _ := os.ReadFile(savePath)
	if string(data) != "cand-small" {
		t.Fatalf("SavePath 内容 = %q, want cand-small（仅成功候选 small 产物）", data)
	}
}

// TestGopeedDownload_MagnetList_AllFail 全部候选失败 → 聚合错误（含每个候选失败原因）。
func TestGopeedDownload_MagnetList_AllFail(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "x.mp4")
	rpc, _ := magnetSrv(t, map[string]bool{}) // 全部候选下载 error
	d := gopeedTestDownloader(rpc, dir)
	obj := &model.DownloadObject{
		SavePath: savePath,
		Extra: map[string]any{
			"magnet_list": []map[string]string{
				{"magnet": "m1", "name": "big", "keepshare": "https://mypikpak.com/s/big", "size": "2GB"},
				{"magnet": "m2", "name": "small", "keepshare": "https://mypikpak.com/s/small", "size": "500MB"},
			},
		},
	}
	err := d.Download(obj, nil)
	if err == nil {
		t.Fatal("expected aggregate error when all magnet candidates fail, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "all 2 magnet candidates failed") {
		t.Errorf("err = %q, want mention 'all 2 magnet candidates failed'", msg)
	}
}

// TestGopeedDownload_SingleShare_NoMagnetList 无 magnet_list 兼容单个分享 URL（obj.URL 走 PikPak 分支）。
func TestGopeedDownload_SingleShare_NoMagnetList(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "out", "movie.mp4")
	if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
		t.Fatal(err)
	}
	prodPath := filepath.Join(filepath.Dir(savePath), "target.mp4")
	if err := os.WriteFile(prodPath, []byte("share-movie"), 0644); err != nil {
		t.Fatal(err)
	}
	rpc, _ := magnetSrv(t, map[string]bool{"share1": true})
	d := gopeedTestDownloader(rpc, dir)
	obj := &model.DownloadObject{
		URL:      "https://mypikpak.com/s/share1",
		SavePath: savePath,
	}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download() error: %v", err)
	}
	data, _ := os.ReadFile(savePath)
	if string(data) != "share-movie" {
		t.Fatalf("SavePath 内容 = %q, want share-movie", data)
	}
}
