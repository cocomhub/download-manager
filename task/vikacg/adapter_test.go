// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package vikacg

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/task"
)

func TestVikacgAdapter_ItemsToURLs(t *testing.T) {
	bt, err := task.NewBaseTask(&config.Task{
		ID:      "test",
		Type:    "vikacg",
		Storage: config.StorageConfig{Type: "memory"},
	}, task.Options{})
	if err != nil {
		t.Fatalf("NewBaseTask failed: %v", err)
	}
	tk := &Task{BaseTask: bt}
	adapter := &vikacgAdapter{t: tk}

	posts := []vikPost{
		{ID: 1001},
		{ID: 0},
		{ID: 1003},
	}
	urls := adapter.ItemsToURLs(posts)
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs (skip ID=0), got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://www.vikacg.com/p/1001" {
		t.Fatalf("unexpected first URL: %s", urls[0])
	}
}

func TestVikacgAdapter_BuildPageURL(t *testing.T) {
	adapter := &vikacgAdapter{}
	u := adapter.BuildPageURL(3)
	if u != "vikacg://internal/page/3" {
		t.Fatalf("unexpected page URL: %s", u)
	}
}

func TestVikacgAdapter_ParseTotalPages(t *testing.T) {
	adapter := &vikacgAdapter{}
	if n := adapter.ParseTotalPages(""); n != -1 {
		t.Fatalf("expected -1 (unknown), got %d", n)
	}
}

func TestVikacgAdapter_ResolveDetail(t *testing.T) {
	adapter := &vikacgAdapter{}
	if err := adapter.ResolveDetail(nil); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestVikacgAdapter_GetDownloadHeaders(t *testing.T) {
	bt, err := task.NewBaseTask(&config.Task{
		ID:      "test",
		Type:    "vikacg",
		Storage: config.StorageConfig{Type: "memory"},
	}, task.Options{})
	if err != nil {
		t.Fatalf("NewBaseTask failed: %v", err)
	}
	tk := &Task{BaseTask: bt, cookie: "test_cookie", userAgent: "test_ua"}
	adapter := &vikacgAdapter{t: tk}

	headers := adapter.GetDownloadHeaders()
	if headers["Cookie"] != "test_cookie" || headers["User-Agent"] != "test_ua" {
		t.Fatalf("unexpected headers: %v", headers)
	}
}