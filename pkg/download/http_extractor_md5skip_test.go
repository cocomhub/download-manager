// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// TestHTTPExtractor_MD5SkipPattern 验证正则白名单 URL 跳过 MD5 校验：
// fourhoi 返回错误 Content-MD5/ETag（与内容不符）→ 无白名单无限重试；
// 配白名单后直接接受文件。
func TestHTTPExtractor_MD5SkipPattern(t *testing.T) {
	t.Parallel()
	// 模拟 fourhoi：返回错误 Content-MD5（与内容不符）。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-MD5", "Wzyi4wI4Bkc+Abm0MPttEQ==") // 错误 MD5
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("fake-jpeg-content-that-differs-from-md5"))
	}))
	defer srv.Close()

	root := t.TempDir()
	// 无白名单：应无限重试后失败（maxRetries=1 快速验证）。
	ex1 := NewHTTPExtractorWithConfig(1, "TestUA", root, "")
	ex1.SetTransport(NewStdlibTransport())
	req1 := &Request{URL: srv.URL + "/cover.jpg", SavePath: root + "/c1.jpg"}
	if err := ex1.Extract(context.Background(), req1); err == nil {
		t.Error("无白名单应失败（MD5 mismatch 重试耗尽）")
	}

	// 有白名单：匹配 fourhoi → 跳过校验 → 成功。
	ex2 := NewHTTPExtractorWithConfig(1, "TestUA", root, "")
	ex2.SetTransport(NewStdlibTransport())
	ex2.SetMd5SkipPatterns([]string{`^https?://127\.0\.0\.1:\d+/cover\.jpg$`})
	req2 := &Request{URL: srv.URL + "/cover.jpg", SavePath: root + "/c2.jpg"}
	if err := ex2.Extract(context.Background(), req2); err != nil {
		t.Fatalf("有白名单应成功: %v", err)
	}
	// 文件应存在非空。
	fi, err := osStat(req2.SavePath)
	if err != nil || fi.Size() == 0 {
		t.Errorf("文件未正确保存: %v size=%v", err, fi)
	}
}

func osStat(p string) (interface{ Size() int64 }, error) {
	return os.Stat(p)
}
