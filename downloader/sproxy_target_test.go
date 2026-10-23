// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
)

// TestWithTargetFileParam 验证 spx_target 叠加语义（空 target 原样、已有 query 保留、覆盖同键）。
func TestWithTargetFileParam(t *testing.T) {
	t.Parallel()
	cases := []struct{ raw, target, wantSub string }{
		{"https://mypikpak.com/s/abc", "NHDTA-513A.mp4", "spx_target=NHDTA-513A.mp4"},
		{"https://mypikpak.com/s/abc?x=1", "b.mp4", "x=1"},
		{"https://keepshare.org/k/magnet:?xt=urn:btih:ab", "m.mp4", "spx_target=m.mp4"},
	}
	for _, c := range cases {
		got := withTargetFileParam(c.raw, c.target)
		if !strings.Contains(got, c.wantSub) {
			t.Fatalf("withTargetFileParam(%q,%q)=%q, want contains %q", c.raw, c.target, got, c.wantSub)
		}
		if c.target != "" && !strings.Contains(got, "spx_target=") {
			t.Fatalf("missing spx_target in %q", got)
		}
		// URL 路径不变
		u1, _ := url.Parse(c.raw)
		u2, _ := url.Parse(got)
		if u1.Path != u2.Path {
			t.Fatalf("path changed: %q -> %q", c.raw, got)
		}
	}
	// 空 target 原样
	if got := withTargetFileParam("https://mypikpak.com/s/x?q=1", ""); got != "https://mypikpak.com/s/x?q=1" {
		t.Fatalf("empty target changed url: %q", got)
	}
}

// TestExtraTargetFileRoundTrip 验证 Extra[target_file] 读写（分卷目标坐标持久化）。
func TestExtraTargetFileRoundTrip(t *testing.T) {
	t.Parallel()
	obj := &model.DownloadObject{URL: "https://mypikpak.com/s/abc"}
	setExtra(obj, extraKeyTargetFile, "NHDTA-513B.mp4")
	if got := extraString(obj, extraKeyTargetFile); got != "NHDTA-513B.mp4" {
		t.Fatalf("target_file = %q, want NHDTA-513B.mp4", got)
	}
}

// TestSproxyCloud_DownloadWithTargetFile 验证分卷目标：Extra[target_file] 叠加到提交 URL
// 的 spx_target（fake sproxy 校验收到的 URL 带参数）。
func TestSproxyCloud_DownloadWithTargetFile(t *testing.T) {
	t.Parallel()
	var gotURL string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotURL = body["url"]
		writeJSONResp(w, map[string]any{"id": "task-9", "status": "running"})
	})
	mux.HandleFunc("GET /api/cloud/tasks/task-9", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-9", "status": "completed"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:    srv.URL + "/api/cloud/download",
		PollEvery: 100,
		CloudOnly: true,
	})
	obj := &model.DownloadObject{
		URL:      "https://mypikpak.com/s/abc",
		SavePath: filepath.Join(t.TempDir(), "out.mp4"),
		Extra: map[string]any{
			"magnet_list": []map[string]string{
				{"keepshare": "https://mypikpak.com/s/abc"},
			},
			"target_file": "NHDTA-513B.mp4",
		},
	}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download error: %v", err)
	}
	if !strings.Contains(gotURL, "spx_target=NHDTA-513B.mp4") {
		t.Fatalf("提交 URL 未带 spx_target: %q", gotURL)
	}
}

// TestSproxyCloud_TransferPathOverride 验证 per-object transfer_path 覆盖静态路径
// （分卷全套每个文件不同卷内路径）：提交请求体的 transfer.path 用 Extra 值。
func TestSproxyCloud_TransferPathOverride(t *testing.T) {
	t.Parallel()
	var gotTransfer map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotTransfer, _ = body["transfer"].(map[string]any)
		writeJSONResp(w, map[string]any{"id": "task-t", "status": "running"})
	})
	mux.HandleFunc("GET /api/cloud/tasks/task-t", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, map[string]any{"id": "task-t", "status": "completed"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:         srv.URL + "/api/cloud/download",
		PollEvery:      100,
		CloudOnly:      true,
		TransferVolume: "jav",
		TransferPath:   "static/path.mp4",
	})
	obj := &model.DownloadObject{
		URL:      "https://mypikpak.com/s/abc",
		SavePath: filepath.Join(t.TempDir(), "out.mp4"),
		Extra: map[string]any{
			"magnet_list":   []map[string]string{{"keepshare": "https://mypikpak.com/s/abc"}},
			"target_file":   "NHDTA-513A.mp4",
			"transfer_path": "jav/makers/natural-high/NHDTC/NHDTC-254/NHDTC-254 就寝中.mp4",
		},
	}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download error: %v", err)
	}
	if gotTransfer == nil {
		t.Fatal("transfer 未提交")
	}
	if gotTransfer["volume"] != "jav" {
		t.Fatalf("volume = %v", gotTransfer["volume"])
	}
	gotPath, _ := gotTransfer["path"].(string)
	if gotPath != "jav/makers/natural-high/NHDTC/NHDTC-254/NHDTC-254 就寝中.mp4" {
		t.Fatalf("path = %q，want per-object 覆盖", gotPath)
	}
}
