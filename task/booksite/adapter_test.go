// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package booksite

import (
	"strings"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/task"
)

func TestBuildPageURL(t *testing.T) {
	a := &booksiteAdapter{}
	if got := a.BuildPageURL(42); got != "booksite://internal/page/42" {
		t.Fatalf("BuildPageURL(42) = %q", got)
	}
}

func TestParsePage(t *testing.T) {
	a := &booksiteAdapter{}
	html := `[{"num":42,"title":"Comic 42","img":"https://imgs.xkcd.com/comics/42.png"}]`
	items, err := a.ParsePage(html)
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}
	comics := items.([]comicInfo)
	if len(comics) != 1 || comics[0].Num != 42 {
		t.Fatalf("ParsePage got %+v", comics)
	}
	// 空页（超出范围）返回空列表
	items, err = a.ParsePage("")
	if err != nil {
		t.Fatalf("ParsePage empty: %v", err)
	}
	if len(items.([]comicInfo)) != 0 {
		t.Fatalf("empty page should return empty list")
	}
}

func newTestTask(t *testing.T) *Task {
	t.Helper()
	cfg := &config.Task{
		ID:   "bt",
		Type: TaskType,
		Storage: config.StorageConfig{
			Type:   "memory",
			Config: map[string]string{},
		},
		Extra: map[string]any{},
	}
	tk, err := NewTask(cfg, task.Options{})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	return tk
}

func TestItemsToURLsAndBuildObject(t *testing.T) {
	tk := newTestTask(t)
	a := &booksiteAdapter{t: tk}
	comics := []comicInfo{{Num: 42, Title: "T", Img: "https://imgs.xkcd.com/comics/42.png"}}
	urls := a.ItemsToURLs(comics)
	if len(urls) != 1 || urls[0] != "https://xkcd.com/42/" {
		t.Fatalf("ItemsToURLs got %v", urls)
	}
	obj, err := a.BuildObject(comics, 0)
	if err != nil {
		t.Fatalf("BuildObject: %v", err)
	}
	if obj.URL != "https://xkcd.com/42/" {
		t.Fatalf("BuildObject URL = %q", obj.URL)
	}
	if obj.GetMetaTitle() != "T" {
		t.Fatalf("BuildObject title = %q", obj.GetMetaTitle())
	}
	if obj.Status != model.StatusPending {
		t.Fatalf("BuildObject status = %q", obj.Status)
	}
	if !strings.Contains(obj.GetMediaPath("cover"), "") && obj.GetMediaPath("cover") != "" {
		t.Fatalf("unexpected cover path")
	}
}
