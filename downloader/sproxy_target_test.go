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

// TestCloudTargets_BSONAndTypedShapes 验证多目标解析兼容内存/mongo 两种形态。
func TestCloudTargets_BSONAndTypedShapes(t *testing.T) {
	t.Parallel()
	typed := &model.DownloadObject{Extra: map[string]any{
		"files": []map[string]string{
			{"url": "https://mypikpak.com/s/abc", "path": "jav/CODE/part1.mp4", "target": "part1.mp4"},
			{"url": "https://mypikpak.com/s/abc", "path": "jav/CODE/part2.mp4", "target": "part2.mp4"},
			{"url": "", "path": "ignored", "target": "empty-url"},
		},
	}}
	got := cloudTargets(typed)
	if len(got) != 2 {
		t.Fatalf("typed 形态应解析 2 条（过滤空 URL），got %d: %+v", len(got), got)
	}
	if got[1].Path != "jav/CODE/part2.mp4" || got[1].Target != "part2.mp4" {
		t.Fatalf("第二条解析错误: %+v", got[1])
	}
	// mongo 往返形态：[]any + map[string]any
	decoded := &model.DownloadObject{Extra: map[string]any{
		"files": []any{
			map[string]any{"url": "https://mypikpak.com/s/abc", "path": "a.mp4", "target": "a.mp4"},
			map[string]any{"url": "https://mypikpak.com/s/abc", "path": "b.srt", "target": "b.srt"},
		},
	}}
	if got := cloudTargets(decoded); len(got) != 2 {
		t.Fatalf("mongo 形态应解析 2 条，got %d", len(got))
	}
}

// TestSproxyCloud_DownloadMultipleTargets 验证**分卷全套**：files 多条 → 每个文件独立
// 提交（各自 spx_target + transfer_path），不再只下载第一个。
// 回归守卫：此前 dm 只提交 targets[0]（多目标名不副实）。
func TestSproxyCloud_DownloadMultipleTargets(t *testing.T) {
	t.Parallel()
	var got []map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cloud/download", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL      string         `json:"url"`
			Transfer map[string]any `json:"transfer"`
			Meta     map[string]any `json:"meta_extra"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		got = append(got, map[string]any{"url": body.URL, "path": body.Transfer["path"], "meta": body.Meta})
		writeJSONResp(w, map[string]any{"id": "task-" + body.Transfer["path"].(string)})
	})
	mux.HandleFunc("GET /api/cloud/tasks/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/cloud/tasks/")
		writeJSONResp(w, map[string]any{
			"id": id, "status": "completed", "filename": id + ".mp4",
			"transfer_url": "secretdata://v/" + id,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := NewSproxyCloudDownloader(config.SproxyCloudConfig{
		APIURL:         srv.URL + "/api/cloud/download",
		PollEvery:      50,
		CloudOnly:      true,
		TransferVolume: "v",
	})
	obj := &model.DownloadObject{
		URL:      "https://mypikpak.com/s/abc",
		SavePath: filepath.Join(t.TempDir(), "out.mp4"),
		Extra: map[string]any{
			// 对象级业务 meta + 逐文件覆盖（share_file_name 逐文件正确，不错配）
			"meta_extra": map[string]any{"code": "EXAMPLE-001", "maker": "M"},
			"files": []map[string]any{
				{"url": "https://mypikpak.com/s/abc", "path": "jav/C/part1.mp4", "target": "part1.mp4",
					"meta": map[string]any{"share_file_name": "part1.mp4", "x_role": "video"}},
				{"url": "https://mypikpak.com/s/abc", "path": "jav/C/part2.mp4", "target": "part2.mp4",
					"meta": map[string]any{"share_file_name": "part2.mp4", "x_role": "subtitle"}},
			},
		},
	}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("应提交 2 个文件，got %d: %+v", len(got), got)
	}
	for i, wantPath := range []string{"jav/C/part1.mp4", "jav/C/part2.mp4"} {
		if got[i]["path"] != wantPath {
			t.Errorf("第 %d 个 transfer.path = %v, want %v", i, got[i]["path"], wantPath)
		}
		if u, _ := got[i]["url"].(string); !strings.Contains(u, "spx_target=part"+string(rune('1'+i))+".mp4") {
			t.Errorf("第 %d 个 URL 缺 spx_target: %v", i, u)
		}
	}
	// 逐文件 meta 合并：对象级 code/maker 保留，文件级 share_file_name 各自正确
	for i, wantName := range []string{"part1.mp4", "part2.mp4"} {
		me, _ := got[i]["meta"].(map[string]any)
		if me["share_file_name"] != wantName {
			t.Errorf("第 %d 个 meta.share_file_name = %v, want %v", i, me["share_file_name"], wantName)
		}
		if me["code"] != "EXAMPLE-001" || me["maker"] != "M" {
			t.Errorf("第 %d 个 meta 丢失对象级业务字段: %v", i, me)
		}
	}
	// 产物坐标：transfer_urls 两条 + 单值兼容键
	urls, _ := obj.Extra["transfer_urls"].([]any)
	if len(urls) != 2 {
		t.Fatalf("transfer_urls 应 2 条，got %d", len(urls))
	}
	if obj.Extra["transfer_url"] == nil {
		t.Fatal("单值 transfer_url 兼容键应写首条")
	}
	if obj.GetProgress() != 100 {
		t.Errorf("progress = %d, want 100", obj.GetProgress())
	}
}
