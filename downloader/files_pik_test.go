// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cocomhub/download-manager/model"
)

func TestGopeedDownload_FilesPikPak(t *testing.T) {
	dir := t.TempDir()
	savePath := filepath.Join(dir, "out", "full.mp4")
	prodPath := filepath.Join(dir, "target.mp4")
	if err := os.WriteFile(prodPath, []byte("movie"), 0644); err != nil {
		t.Fatal(err)
	}
	files := []map[string]any{
		{"name": "SAMPLE-123-uncensored-HD.mp4", "size": 100, "req": map[string]any{"url": "https://dl.mypikpak.com/download/?fid=zzz"}},
	}
	rpc := gopeedPikPakServer(t, files)
	d := gopeedTestDownloader(rpc, dir)
	// obj.URL 是详情页（非 pikpak），但 files 里含 keepshare → 应走 PikPak 分支
	obj := &model.DownloadObject{
		URL: "https://njavtv.com/ja/sample-123",
		Extra: map[string]any{
			"files": []map[string]string{{"url": rpc + "/keepshare?dn=SAMPLE-123-uncensored-HD"}},
		},
		SavePath: savePath,
	}
	if err := d.Download(obj, nil); err != nil {
		t.Fatalf("Download(): %v", err)
	}
	if _, err := os.Stat(savePath); err != nil {
		t.Errorf("产物未到 %s: %v", savePath, err)
	}
}
