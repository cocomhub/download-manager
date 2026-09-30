// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/model"
)

// TestMoveResult_FindsLatestFile 验证 files[0].name 不存在（Gopeed 加 '(1)' 后缀）
// 时，moveResult 扫描目录最新文件作为产物。
func TestMoveResult_FindsLatestFile(t *testing.T) {
	dir := t.TempDir()
	saveDir := filepath.Join(dir, "SERIES")
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		t.Fatal(err)
	}
	// 旧文件 download（非产物）+ 新文件 download (1)（实际产物）
	oldFile := filepath.Join(saveDir, "download")
	if err := os.WriteFile(oldFile, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-10 * time.Minute)
	os.Chtimes(oldFile, oldTime, oldTime)
	prod := filepath.Join(saveDir, "download (1)")
	if err := os.WriteFile(prod, []byte("new-product"), 0644); err != nil {
		t.Fatal(err)
	}

	d := &GopeedDownloader{downloadDir: dir}
	// 直接测 latestFileInDir（产物名冲突场景：最新修改文件即产物）
	latest := d.latestFileInDir(&model.DownloadObject{SavePath: filepath.Join(saveDir, "movie.mp4")})
	if latest != prod {
		t.Fatalf("latestFileInDir = %q, want %q (最新文件)", latest, prod)
	}
}
