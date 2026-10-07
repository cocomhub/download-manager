// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// runTest 调用 run()，返回退出码与输出。
func runTest(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var so, se bytes.Buffer
	code := run(args, &so, &se)
	return code, so.String(), se.String()
}

func TestRun_NoArgs(t *testing.T) {
	code, _, se := runTest(t)
	if code != exitUsage {
		t.Fatalf("code = %d, want %d; stderr=%s", code, exitUsage, se)
	}
}

func TestRun_TooManyArgs(t *testing.T) {
	code, _, _ := runTest(t, "http://a", "http://b")
	if code != exitUsage {
		t.Fatalf("code = %d, want %d", code, exitUsage)
	}
}

func TestRun_UnsupportedURL(t *testing.T) {
	code, _, se := runTest(t, "magnet:?xt=urn:btih:ABC")
	if code != exitFail {
		t.Fatalf("code = %d, want %d", code, exitFail)
	}
	if !bytes.Contains([]byte(se), []byte("暂不支持")) {
		t.Fatalf("stderr = %q, want 暂不支持", se)
	}
}

func TestRun_Version(t *testing.T) {
	code, so, _ := runTest(t, "--version")
	if code != exitOK {
		t.Fatalf("code = %d, want %d", code, exitOK)
	}
	if !bytes.Contains([]byte(so), []byte("pikget ")) {
		t.Fatalf("stdout = %q, want pikget version", so)
	}
}

func TestRun_DirectDownloadSuccess(t *testing.T) {
	content := []byte("pikget e2e via run()")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(content)
	}))
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "out.bin")
	code, so, se := runTest(t, "-o", dest, srv.URL+"/f.bin")
	if code != exitOK {
		t.Fatalf("code = %d, want %d; stderr=%s", code, exitOK, se)
	}
	if !bytes.Contains([]byte(so), []byte("done")) {
		t.Fatalf("stdout = %q, want done", so)
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, content) {
		t.Fatalf("content mismatch: got %q", got)
	}
}

func TestRun_DirectDownloadServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	code, _, se := runTest(t, srv.URL+"/err.bin")
	// 不指定 -o，会写到当前目录（污染）；显式给 -o 到临时目录
	_ = se
	_ = code
	dest := filepath.Join(t.TempDir(), "o.bin")
	code, _, se = runTest(t, "-o", dest, srv.URL+"/err.bin")
	if code != exitFail {
		t.Fatalf("code = %d, want %d; stderr=%s", code, exitFail, se)
	}
}

func TestRun_QuietNoProgress(t *testing.T) {
	content := []byte("quiet download")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "q.bin")
	code, so, se := runTest(t, "-q", "-o", dest, srv.URL+"/q.bin")
	if code != exitOK {
		t.Fatalf("code = %d, want %d; stderr=%s", code, exitOK, se)
	}
	if so != "" {
		t.Fatalf("quiet mode should print nothing, got: %q", so)
	}
}
