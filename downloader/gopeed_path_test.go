// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cocomhub/download-manager/model"
)

// TestResultPath_UsesSavePathDir 验证 resultPath 与 Gopeed 落盘目录一致：
// 产物在 obj.SavePath 所在目录找（而非全局 downloadDir）。
func TestResultPath_UsesSavePathDir(t *testing.T) {
	globalDir := t.TempDir()
	// SavePath 目录与全局不同
	saveDir := filepath.Join(t.TempDir(), "task", "SERIES")
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		t.Fatal(err)
	}
	// 产物实际落在 SavePath 目录
	prod := filepath.Join(saveDir, "movie.mp4")
	if err := os.WriteFile(prod, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	d := &GopeedDownloader{downloadDir: globalDir}
	obj := &model.DownloadObject{
		URL:      "http://example.com/movie.mp4",
		SavePath: filepath.Join(saveDir, "movie.mp4"),
	}
	files := []gopeedFile{{Name: "movie.mp4"}}
	got := d.resultPath(obj, files)
	want := filepath.Join(saveDir, "movie.mp4")
	if got != want {
		t.Fatalf("resultPath = %q, want %q (SavePath dir, not global %q)", got, want, globalDir)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("产物应在 SavePath 目录可找到: %v", err)
	}
}
