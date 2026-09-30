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

	"github.com/cocomhub/download-manager/model"
)

// gopeedProgressServer 返回带进度字段的 mock：首次 running(downloaded=100)，后 done。
func gopeedProgressServer(t *testing.T, statusFile string) *GopeedDownloader {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
			writeGopeedResponse(w, "t-prog")
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks/t-prog":
			n := calls.Add(1)
			var task gopeedTask
			if n == 1 {
				task = gopeedTask{ID: "t-prog", Status: "running", Progress: gopeedProgress{Downloaded: 100, Total: 1000, Speed: 50}}
			} else {
				task = gopeedTask{ID: "t-prog", Status: "done", Meta: gopeedMeta{Res: gopeedRes{Files: []gopeedFile{{Name: "movie.mp4"}}}}}
			}
			writeGopeedResponse(w, task)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	d := gopeedTestDownloader(srv.URL, dir)
	d.status = newStatusWriter(statusFile)
	return d
}

// TestGopeedStatus_Persisted 验证轮询进度 + done 终态写入状态文件。
func TestGopeedStatus_Persisted(t *testing.T) {
	sf := filepath.Join(t.TempDir(), "gopeed-status.json")
	d := gopeedProgressServer(t, sf)
	obj := &model.DownloadObject{URL: "magnet:?xt=urn:btih:abcdef1234567890abcdef1234567890", SavePath: filepath.Join(d.downloadDir, "out", "movie.mp4")}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download(): %v", err)
	}
	data, err := os.ReadFile(sf)
	if err != nil {
		t.Fatalf("status file missing: %v", err)
	}
	// NDJSON：多行追加，逐行解析取最后一条（done 终态）
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected ≥2 appended lines (NDJSON), got %d: %q", len(lines), string(data))
	}
	var st GopeedTaskState
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &st); err != nil {
		t.Fatalf("bad last status json: %v", err)
	}
	if st.TaskID != "t-prog" || st.Status != "done" || st.URL != obj.URL || st.SavePath != obj.SavePath {
		t.Fatalf("status = %+v", st)
	}
	if st.Downloaded == 0 || st.UpdatedAt.IsZero() {
		t.Fatalf("progress/updated not recorded: %+v", st)
	}
}

// TestGopeedStatus_Error 验证 error 终态记录错误信息。
func TestGopeedStatus_Error(t *testing.T) {
	sf := filepath.Join(t.TempDir(), "gopeed-status-err.json")
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
			writeGopeedResponse(w, "t-err")
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks/t-err":
			calls.Add(1)
			writeGopeedResponse(w, gopeedTask{ID: "t-err", Status: "error"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	d := gopeedTestDownloader(srv.URL, t.TempDir())
	d.status = newStatusWriter(sf)
	obj := &model.DownloadObject{URL: "magnet:?xt=urn:btih:abcdef1234567890abcdef1234567890", SavePath: filepath.Join(d.downloadDir, "x")}
	_ = d.Download(obj, nil) // 返回 error，不关心
	data, err := os.ReadFile(sf)
	if err != nil {
		t.Fatalf("status file missing on error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatalf("no status lines: %q", string(data))
	}
	var st GopeedTaskState
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &st); err != nil {
		t.Fatalf("bad status json: %v", err)
	}
	if st.Status != "error" || st.Error == "" {
		t.Fatalf("error status not recorded: %+v", st)
	}
}
