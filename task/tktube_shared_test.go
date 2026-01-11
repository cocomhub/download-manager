package task_test

import (
	"testing"

	"download-manager/manager"
	"download-manager/model"
	"download-manager/storage"
	"download-manager/task"
)

func TestSharedURLStateAcrossTasks(t *testing.T) {
	reg := manager.NewURLStateRegistry()

	store1, _ := storage.NewMemoryStorage(nil)
	store2, _ := storage.NewMemoryStorage(nil)

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
	if err := t1.UpdateStatus(objs1[0], model.StatusCompleted, nil); err != nil {
		t.Fatalf("update status failed: %v", err)
	}

	// Fetch via task2 to trigger sync from shared registry
	_, _ = t2.GetDownloadObjects()
	// Then verify via full list
	all2 := t2.GetAllObjects()
	if len(all2) != 1 {
		t.Fatalf("expected 1 object for task2, got %d", len(all2))
	}
	if all2[0].Status != model.StatusCompleted {
		t.Fatalf("expected status completed via shared registry, got %s", all2[0].Status)
	}
}
