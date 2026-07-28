// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
)

func TestUpdateObjectTags_Success(t *testing.T) {
	t.Parallel()
	st, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatalf("NewMemoryStorage: %v", err)
	}
	obj := &model.DownloadObject{
		TaskID: "t1",
		URL:    "https://example.com/video",
		Extra:  map[string]any{"tags": []string{"old1", "old2"}},
	}
	obj.SetID(1)
	if err := st.Update(obj); err != nil {
		t.Fatalf("failed to store object: %v", err)
	}

	m := NewManager(&config.Config{})
	m.tasks.Store("t1", &mockTaskWithStorage{id: "t1", typ: "hanime", st: st})

	newTags := []string{"new1", "new2", "new3"}
	if err := m.UpdateObjectTags("hanime", 1, newTags); err != nil {
		t.Fatalf("UpdateObjectTags failed: %v", err)
	}

	updated, err := m.GetObjectByTypeAndID("hanime", 1)
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil {
		t.Fatal("object not found")
	}
	got := updated.GetTags()
	if len(got) != 3 || got[0] != "new1" || got[1] != "new2" || got[2] != "new3" {
		t.Fatalf("tags = %v, want [new1 new2 new3]", got)
	}
}

func TestUpdateObjectTags_EmptyTags(t *testing.T) {
	t.Parallel()
	st, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatalf("NewMemoryStorage: %v", err)
	}
	obj := &model.DownloadObject{
		TaskID: "t1",
		URL:    "https://example.com/video",
		Extra:  map[string]any{"tags": []string{"old1", "old2"}},
	}
	obj.SetID(1)
	if err := st.Update(obj); err != nil {
		t.Fatalf("failed to store object: %v", err)
	}

	m := NewManager(&config.Config{})
	m.tasks.Store("t1", &mockTaskWithStorage{id: "t1", typ: "hanime", st: st})

	if err := m.UpdateObjectTags("hanime", 1, []string{}); err != nil {
		t.Fatalf("UpdateObjectTags failed: %v", err)
	}

	updated, err := m.GetObjectByTypeAndID("hanime", 1)
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil {
		t.Fatal("object not found")
	}
	got := updated.GetTags()
	if len(got) != 0 {
		t.Fatalf("tags = %v, want empty", got)
	}
}

func TestUpdateObjectTags_ObjectNotFound(t *testing.T) {
	t.Parallel()
	m := NewManager(&config.Config{})
	m.tasks.Store("t1", &mockTaskWithStorage{id: "t1", typ: "hanime"})

	err := m.UpdateObjectTags("hanime", 999, []string{"tag1"})
	if err == nil {
		t.Fatal("expected error for non-existent object, got nil")
	}
}

func TestUpdateObjectTags_TaskTypeNotFound(t *testing.T) {
	t.Parallel()
	m := NewManager(&config.Config{})

	err := m.UpdateObjectTags("nonexistent", 1, []string{"tag1"})
	if err == nil {
		t.Fatal("expected error for non-existent task type, got nil")
	}
}

// Ensure mockTaskWithStorage implements core.Task (compile-time check).
var _ core.Task = (*mockTaskWithStorage)(nil)
