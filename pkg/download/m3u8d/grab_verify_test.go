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

// TestExtractMD5ETag 验证通过 pkg/download.TryGetMd5 提取 ETag 内容 MD5 的逻辑：
// farward. TryGetMd5 对强/弱 ETag（内容为 32 位 hex）返回 hexMD5；非 MD5 形态返回空。
func TestExtractMD5ETag(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"强 ETag 裸 32hex", `"81E917CBDB4C9F3C1CA89B2CB9842F84"`, "81E917CBDB4C9F3C1CA89B2CB9842F84"},
		{"弱 ETag W/", `W/"81E917CBDB4C9F3C1CA89B2CB9842F84"`, "81E917CBDB4C9F3C1CA89B2CB9842F84"},
		{"非 32 位长度", `"abc123"`, ""},
		{"opaque 强 ETag", `"v1"`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := download.TryGetMd5(map[string]string{"Etag": c.in})
			if got != c.want {
				t.Fatalf("TryGetMd5(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// newVerifyTestServer 返回一个分片下载服务器。
// 每次请求返回 body=segData，并据 etag 参数控制返回的 ETag 头：
//   - "none"      不设置 ETag
//   - "match"     ETag = 内容真实 MD5
//   - "mismatch"  ETag = 与内容不符的 MD5（内容每次不变 → 重下内容一致 → 最终通过）
//   - "opaque"    ETag = 非 MD5 形态头
//
// redownloadDifferent=true 时每次请求返回递增计数后缀 → 重下内容总不同 → 校验失败。
func newVerifyTestServer(t *testing.T, segData []byte, etagMode string, redownloadDifferent bool) *httptest.Server {
	t.Helper()
	sum := md5.Sum(segData)
	hexMD5 := hex.EncodeToString(sum[:])
	var count atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/seg.ts", func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		w.Header().Set("Content-Type", "application/octet-stream")
		switch etagMode {
		case "match":
			w.Header().Set("ETag", `"`+hexMD5+`"`)
		case "mismatch":
			w.Header().Set("ETag", `"00000000000000000000000000000000"`)
		case "opaque":
			w.Header().Set("ETag", `W/"v1"`)
		}
		body := segData
		if redownloadDifferent {
			body = []byte(fmt.Sprintf("content-%d", n))
		}
		_, _ = w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func verifyDummyEngine(dir string, disable bool) *M3U8DEngine {
	return &M3U8DEngine{
		Config: &DownloadConfig{
			WorkDir:           dir,
			DisableVerifyETag: disable,
		},
		downloaded: make(map[string]bool),
	}
}

func runVerifyCase(t *testing.T, disable bool, etagMode string) error {
	return runVerifyCaseFull(t, disable, etagMode, false)
}

func runVerifyCaseFull(t *testing.T, disable bool, etagMode string, redownloadDifferent bool) error {
	t.Helper()
	data := []byte("hello-segment-219")
	srv := newVerifyTestServer(t, data, etagMode, redownloadDifferent)
	dir := t.TempDir()
	d := verifyDummyEngine(dir, disable)
	d.verifyMD5 = sync.Map{}
	tasks := []DownloadTask{
		{URL: srv.URL + "/seg.ts", LocalPath: filepath.Join(dir, "seg.ts"), Type: "ts"},
	}
	return d.downloadFilesConcurrently(t.Context(), tasks)
}

func TestVerifyETag_Disabled(t *testing.T) {
	// DisableVerifyETag=true：显式禁用校验，即使 ETag 与内容不匹配也通过。
	if err := runVerifyCase(t, true, "mismatch"); err != nil {
		t.Fatalf("expected pass when verify disabled, got: %v", err)
	}
}

func TestVerifyETag_Match(t *testing.T) {
	// 默认开启校验（DisableVerifyETag=false），ETag=内容 MD5 直通。
	if err := runVerifyCase(t, false, "match"); err != nil {
		t.Fatalf("ETag match should pass, got: %v", err)
	}
}

func TestVerifyETag_MismatchRedownloadIdenticalPasses(t *testing.T) {
	// 默认开启：校验不匹配（ETag 与内容不符，但内容每次一致）→ 重下后内容相同 → 接受。
	// 覆盖：新域名 ETag 非内容 MD5 时不被误拦。
	if err := runVerifyCaseFull(t, false, "mismatch", false); err != nil {
		t.Fatalf("mismatch with stable content should pass after redownload, got: %v", err)
	}
}

func TestVerifyETag_MismatchRedownloadDifferentFails(t *testing.T) {
	// 默认开启：重下后内容不同（中间变异/损坏）且仍与 ETag 不符 → 应失败。
	err := runVerifyCaseFull(t, false, "mismatch", true)
	if err == nil {
		t.Fatal("expected failure when redownload content differs and ETag mismatch")
	}
}

func TestVerifyETag_NonMD5RedownloadPasses(t *testing.T) {
	// 默认开启但 ETag 非 MD5 形态：走两致对接——内容稳定时重下一次通过。
	if err := runVerifyCase(t, false, "opaque"); err != nil {
		t.Fatalf("non-MD5 ETag with stable content should pass via redownload, got: %v", err)
	}
}

func TestVerifyETag_NoETagRedownloadPasses(t *testing.T) {
	// 默认开启且 ETag 缺失：同样走两致对接，内容稳定时通过。
	if err := runVerifyCase(t, false, "none"); err != nil {
		t.Fatalf("missing ETag with stable content should pass via redownload, got: %v", err)
	}
}

func TestVerifyETag_NoETagDifferentFails(t *testing.T) {
	// 默认开启且 ETag 缺失、每次内容不同（弱网）→ 永不出现两份相同 → 失败。
	err := runVerifyCaseFull(t, false, "none", true)
	if err == nil {
		t.Fatal("expected failure when no ETag and content keeps changing")
	}
}

// TestVerifyETag_NoHTTPResponse 防御：resp 无 HTTPResponse 时不 panic、不校验。
func TestVerifyETag_NoHTTPResponse(t *testing.T) {
	d := verifyDummyEngine(t.TempDir(), false)
	retry, err := d.verifySegment(nil)
	if err != nil {
		t.Fatalf("nil resp should not error, got: %v", err)
	}
	if retry {
		t.Fatal("nil resp should not trigger retry")
	}
}
