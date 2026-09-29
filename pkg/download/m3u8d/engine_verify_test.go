// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package m3u8d

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

// newReliableServer 单文件下载服务器：
// mode=mismatch 时 ETag 与内容不符（内容每次相同 → 两次相同即通过）；
// mode=stableWrong 时内容每次不同（永不出现两份相同 → 轮次耗尽失败）。
func newReliableServer(t *testing.T, mode string) *httptest.Server {
	t.Helper()
	var n atomic.Int64
	good := []byte("single-file-good")
	sum := md5.Sum(good)
	hexMD5 := hex.EncodeToString(sum[:])
	mux := http.NewServeMux()
	mux.HandleFunc("/file.bin", func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("ETag", `"`+hexMD5+`"`)
		var body []byte
		switch mode {
		case "match":
			body = good
		case "stableWrong":
			body = []byte(fmt.Sprintf("changing-%d", n.Load()))
		default: // mismatch-stable
			body = good
			w.Header().Set("ETag", `"00000000000000000000000000000000"`)
		}
		_, _ = w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func runReliable(t *testing.T, mode string, disable bool) error {
	t.Helper()
	srv := newReliableServer(t, mode)
	dir := t.TempDir()
	d := &M3U8DEngine{
		Config: &DownloadConfig{
			WorkDir:           dir,
			DisableVerifyETag: disable,
			MaxRetries:        3,
		},
		downloaded: make(map[string]bool),
		verifyMD5:  sync.Map{},
	}
	d.client = srv.Client()
	return d.DownloadFileReliable(t.Context(), srv.URL+"/file.bin", filepath.Join(dir, "file.bin"))
}

func TestDownloadFileReliable_Match(t *testing.T) {
	if err := runReliable(t, "match", false); err != nil {
		t.Fatalf("match should pass, got: %v", err)
	}
}

func TestDownloadFileReliable_MismatchStablePasses(t *testing.T) {
	// ETag 与内容不符但内容稳定：两次相同 → 接受（默认开启校验）。
	if err := runReliable(t, "mismatch-stable", false); err != nil {
		t.Fatalf("stable wrong etag should pass after two identical, got: %v", err)
	}
}

func TestDownloadFileReliable_ContentKeepsChangingFails(t *testing.T) {
	// 每次内容都不同（弱网/变异）：永不出现两份相同 → 轮次耗尽失败。
	err := runReliable(t, "stableWrong", false)
	if err == nil {
		t.Fatal("expected failure when content keeps changing")
	}
}

func TestDownloadFileReliable_VerifyDisabledSkips(t *testing.T) {
	// 显式禁用校验（DisableVerifyETag=true）：单文件路径不做两致对接，直接通过。
	if err := runReliable(t, "stableWrong", true); err != nil {
		t.Fatalf("verify disabled should pass without checks, got: %v", err)
	}
}

var _ = download.MD5HexEqual
