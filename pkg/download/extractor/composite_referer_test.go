// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

// TestCompositeExtractor_RefererHeader 验证 composite 子下载带 files 条目的 referer。
// 实测：surrit.com m3u8 无 Referer → 403；带 Referer → 200（防盗链）。
func TestCompositeExtractor_RefererHeader(t *testing.T) {
	t.Parallel()
	var gotRef atomic.Value
	// 目标：验证 Referer 头（无 → 403，有 → 200）。
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		gotRef.Store(r.Header.Get("Referer"))
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("video-data"))
	}))
	defer target.Close()

	ex := NewCompositeExtractor()
	ex.AddExtractor(download.NewHTTPExtractor())
	filesJSON, _ := json.Marshal([]map[string]string{
		{
			"type": "video", "url": target.URL + "/video.mp4",
			"path":    filepath.ToSlash(t.TempDir()) + "/out.mp4",
			"referer": "https://njavtv.com/ja/nhdtc-test",
		},
	})
	req := &download.Request{
		URL:      "njavtv://composite/" + target.URL,
		SavePath: filepath.ToSlash(t.TempDir()) + "/parent.mp4",
		Metadata: map[string]string{
			"files": string(filesJSON),
		},
	}
	if err := ex.Extract(context.Background(), req); err != nil {
		t.Fatalf("Extract err: %v", err)
	}
	ref, _ := gotRef.Load().(string)
	if !strings.Contains(ref, "njavtv.com") {
		t.Errorf("Referer = %q, want njavtv.com 详情页", ref)
	}
}
