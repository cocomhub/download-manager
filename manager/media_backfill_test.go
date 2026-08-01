// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
)

// mediaTask 实现 core.Task + core.SmallObjectProvider，使用内存存储。
type mediaTask struct {
	*mockTask
	store core.Storage
}

func (m *mediaTask) Storage() core.Storage { return m.store }
func (m *mediaTask) Type() string          { return "media-task" }

func (m *mediaTask) SmallObjects(obj *model.DownloadObject) []core.SmallObjectInfo {
	if obj == nil || obj.Extra == nil {
		return nil
	}
	if u, _ := obj.Extra["thumb_url"].(string); u != "" {
		return []core.SmallObjectInfo{{
			URL:      u,
			SavePath: "/downloads/" + obj.URL + "_thumb.jpg",
			Rel:      model.MediaRelThumb,
		}}
	}
	return nil
}

// TestRunMediaBackfill 验证初始化媒体回填：把 {rel}_url / {rel}_path 写入所有对象。
func TestRunMediaBackfill(t *testing.T) {
	cfg := &config.Config{Tasks: []config.Task{{ID: "t1", Type: "media-task"}}}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)

	store, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatalf("NewMemoryStorage: %v", err)
	}
	o1 := &model.DownloadObject{URL: "u1", Extra: map[string]any{"thumb_url": "https://x/t1.jpg"}}
	o2 := &model.DownloadObject{URL: "u2", Extra: map[string]any{}} // 无媒体 → 不应写入
	if err := store.Update(o1); err != nil {
		t.Fatalf("seed o1: %v", err)
	}
	if err := store.Update(o2); err != nil {
		t.Fatalf("seed o2: %v", err)
	}

	m.tasks.Store("t1", &mediaTask{mockTask: &mockTask{id: "t1", typ: "media-task"}, store: store})

	NewStandardizationService(m).runMediaBackfill(context.Background())

	got1, err := store.Get("u1")
	if err != nil {
		t.Fatalf("Get u1: %v", err)
	}
	if got1.GetMediaURL(model.MediaRelThumb) != "https://x/t1.jpg" {
		t.Errorf("u1 thumb_url = %q", got1.GetMediaURL(model.MediaRelThumb))
	}
	if got1.GetMediaPath(model.MediaRelThumb) != "/downloads/u1_thumb.jpg" {
		t.Errorf("u1 thumb_path = %q", got1.GetMediaPath(model.MediaRelThumb))
	}

	got2, err := store.Get("u2")
	if err != nil {
		t.Fatalf("Get u2: %v", err)
	}
	if got2.GetMediaURL(model.MediaRelThumb) != "" || got2.GetMediaPath(model.MediaRelThumb) != "" {
		t.Errorf("u2 should not have media fields, got %q / %q",
			got2.GetMediaURL(model.MediaRelThumb), got2.GetMediaPath(model.MediaRelThumb))
	}
}
