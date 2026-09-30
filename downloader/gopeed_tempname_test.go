// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cocomhub/download-manager/model"
)

// TestTempName_DefaultSuffix 验证中间文件名 = SavePath 文件名 + .download。
func TestTempName_DefaultSuffix(t *testing.T) {
	d := &GopeedDownloader{}
	obj := &model.DownloadObject{SavePath: "/data/SERIES/movie.mp4"}
	got := d.tempName(obj)
	if got != "movie.mp4.download" {
		t.Fatalf("tempName = %q, want movie.mp4.download", got)
	}
}

// TestTempName_CustomSuffix 验证可配置后缀。
func TestTempName_CustomSuffix(t *testing.T) {
	d := &GopeedDownloader{tempSuffix: ".part"}
	obj := &model.DownloadObject{SavePath: "/data/SERIES/movie.mp4"}
	if got := d.tempName(obj); got != "movie.mp4.part" {
		t.Fatalf("tempName = %q, want movie.mp4.part", got)
	}
}

// TestMoveResult_TempSuffixRename 验证：Gopeed 落盘 <name>.download → moveResult 改名为最终 SavePath。
func TestMoveResult_TempSuffixRename(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "SERIES", "movie.mp4")
	if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
		t.Fatal(err)
	}
	// Gopeed 落盘中间名（tempName）
	tempPath := filepath.Join(filepath.Dir(savePath), "movie.mp4.download")
	if err := os.WriteFile(tempPath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	d := &GopeedDownloader{downloadDir: dir}
	// 直接测 moveResult 的定位逻辑（tempResultPath）
	got := d.tempResultPath(&model.DownloadObject{SavePath: savePath})
	if got != tempPath {
		t.Fatalf("tempResultPath = %q, want %q", got, tempPath)
	}
	if !fileExists(got) {
		t.Fatalf("temp file should exist: %q", got)
	}
}
