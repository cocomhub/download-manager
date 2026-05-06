// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task_test

import (
	"testing"

	"github.com/cocomhub/download-manager/manager"
	"github.com/cocomhub/download-manager/pkg/dlcore"
	"github.com/cocomhub/download-manager/storage"
	"github.com/cocomhub/download-manager/task"
)

func TestSharedURLStateAcrossTasks(t *testing.T) {
	reg := manager.NewURLStateRegistry()

	store1, _ := storage.NewMemoryStorage(nil)
	store2, _ := storage.NewMemoryStorage(nil)
	reg.RegisterStorage("task1", store1)
	reg.RegisterStorage("task2", store2)

	urls := []string{"http://example.com/file.mp4"}

	t1 := task.NewSimpleTask("task1", urls, "/tmp/save1", store1)
	t1.SetSharedRegistry(reg)

	t2 := task.NewSimpleTask("task2", urls, "/tmp/save2", store2)
	t2.SetSharedRegistry(reg)

	objs1, _ := t1.GetDownloadObjects()
	if len(objs1) != 1 {
		t.Fatalf("expected 1 object for task1, got %d", len(objs1))
	}

	// Update status via task1
	if err := t1.UpdateStatus(objs1[0], dlcore.StatusCompleted, nil); err != nil {
		t.Fatalf("update status failed: %v", err)
	}

	// Fetch via task2 to trigger sync from shared registry
	_, _ = t2.GetDownloadObjects()
	// Then verify via full list
	all2 := t2.GetAllObjects()
	if len(all2) != 1 {
		t.Fatalf("expected 1 object for task2, got %d", len(all2))
	}
	if all2[0].Status != dlcore.StatusCompleted {
		t.Fatalf("expected status completed via shared registry, got %s", all2[0].Status)
	}
}

func TestSharedURLStateLazyHydratesFromStorageOnMiss(t *testing.T) {
	reg := manager.NewURLStateRegistry()

	store1, _ := storage.NewMemoryStorage(nil)
	store2, _ := storage.NewMemoryStorage(nil)
	reg.RegisterStorage("task1", store1)
	reg.RegisterStorage("task2", store2)

	urls := []string{"http://example.com/file.mp4"}
	t1 := task.NewSimpleTask("task1", urls, "/tmp/save1", store1)
	t1.SetSharedRegistry(reg)
	t2 := task.NewSimpleTask("task2", urls, "/tmp/save2", store2)
	t2.SetSharedRegistry(reg)

	objs1, _ := t1.GetDownloadObjects()
	if len(objs1) != 1 {
		t.Fatalf("expected 1 object for task1, got %d", len(objs1))
	}
	if err := store1.Update(objs1[0]); err != nil {
		t.Fatalf("seed storage failed: %v", err)
	}
	if err := t1.UpdateStatus(objs1[0], dlcore.StatusCompleted, nil); err != nil {
		t.Fatalf("update status failed: %v", err)
	}

	// Simulate a cold manager start where the shared registry is empty and must
	// lazily hydrate from task storage instead of a startup full scan.
	reg = manager.NewURLStateRegistry()
	reg.RegisterStorage("task1", store1)
	reg.RegisterStorage("task2", store2)
	t2.SetSharedRegistry(reg)

	_, _ = t2.GetDownloadObjects()
	all2 := t2.GetAllObjects()
	if len(all2) != 1 {
		t.Fatalf("expected 1 object for task2, got %d", len(all2))
	}
	if all2[0].Status != dlcore.StatusCompleted {
		t.Fatalf("expected lazy hydrated completed status, got %s", all2[0].Status)
	}
	if owners := reg.Owners(urls[0]); owners != 1 {
		t.Fatalf("expected owners=1 after lazy hydration, got %d", owners)
	}
}
