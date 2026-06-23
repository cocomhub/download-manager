// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task_test

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/manager"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/task"
	"github.com/cocomhub/download-manager/task/urllist"
)

func NewTestTask(t *testing.T, id string, urls []string) core.Task {
	tt, err := task.NewTask(&config.Task{
		ID:      id,
		Type:    urllist.TaskType,
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{
			Type: "memory",
		},
		Extra: map[string]any{"urls": urls},
	})
	if err != nil {
		t.Fatalf("new task err: %s", err)
	}
	return tt
}

func TestSharedURLStateAcrossTasks(t *testing.T) {
	urls := []string{"http://example.com/file.mp4"}

	t1 := NewTestTask(t, "task1", urls)
	t2 := NewTestTask(t, "task2", urls)

	reg := manager.NewURLStateRegistry()
	reg.RegisterStorage(t1.ID(), t1.Storage())
	reg.RegisterStorage(t2.ID(), t2.Storage())

	t1.(*urllist.Task).SetSharedRegistry(reg)
	t2.(*urllist.Task).SetSharedRegistry(reg)

	objs1, _ := t1.GetDownloadObjects()
	if len(objs1) != 1 {
		t.Fatalf("expected 1 object for task1, got %d", len(objs1))
	}

	// Update status via task1
	if err := t1.UpdateStatus(objs1[0], model.StatusCompleted, nil); err != nil {
		t.Fatalf("update status failed: %v", err)
	}

	// Fetch via task2 to trigger sync from shared registry
	_, _ = t2.GetDownloadObjects()
	// Then verify via full list
	all2 := t2.(*urllist.Task).GetAllObjects(true)
	if len(all2) != 1 {
		t.Fatalf("expected 1 object for task2, got %d", len(all2))
	}
	if all2[0].GetStatus() != model.StatusCompleted {
		t.Fatalf("expected status completed via shared registry, got %s", all2[0].Status)
	}
}

func TestSharedURLStateLazyHydratesFromStorageOnMiss(t *testing.T) {
	urls := []string{"http://example.com/file.mp4"}
	t1 := NewTestTask(t, "task1", urls)
	t2 := NewTestTask(t, "task2", urls)

	reg := manager.NewURLStateRegistry()
	reg.RegisterStorage(t1.ID(), t1.Storage())
	reg.RegisterStorage(t2.ID(), t2.Storage())

	t1.(*urllist.Task).SetSharedRegistry(reg)
	t2.(*urllist.Task).SetSharedRegistry(reg)

	objs1, _ := t1.GetDownloadObjects()
	if len(objs1) != 1 {
		t.Fatalf("expected 1 object for task1, got %d", len(objs1))
	}
	if err := t1.Storage().Update(objs1[0]); err != nil {
		t.Fatalf("seed storage failed: %v", err)
	}
	if err := t1.UpdateStatus(objs1[0], model.StatusCompleted, nil); err != nil {
		t.Fatalf("update status failed: %v", err)
	}

	// Simulate a cold manager start where the shared registry is empty and must
	// lazily hydrate from task storage instead of a startup full scan.
	reg = manager.NewURLStateRegistry()
	reg.RegisterStorage(t1.ID(), t1.Storage())
	reg.RegisterStorage(t2.ID(), t2.Storage())
	t2.(*urllist.Task).SetSharedRegistry(reg)

	_, _ = t2.GetDownloadObjects()
	all2 := t2.(*urllist.Task).GetAllObjects(true)
	if len(all2) != 1 {
		t.Fatalf("expected 1 object for task2, got %d", len(all2))
	}
	if all2[0].GetStatus() != model.StatusCompleted {
		t.Fatalf("expected lazy hydrated completed status, got %s", all2[0].Status)
	}
	if owners := reg.Owners(urls[0]); owners != 1 {
		t.Fatalf("expected owners=1 after lazy hydration, got %d", owners)
	}
}
