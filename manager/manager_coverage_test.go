// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sync"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// =============================================================================
// GetDownloadRootDir
// =============================================================================

func TestGetDownloadRootDir_FromDownloadRootDir(t *testing.T) {
	rootDir := t.TempDir()
	cfg := &config.Config{
		Server: config.Server{
			DownloadRootDir: rootDir,
		},
	}
	m := NewManager(cfg)

	got := m.GetDownloadRootDir()
	if got != rootDir {
		t.Fatalf("GetDownloadRootDir() = %q, want %q", got, rootDir)
	}
}

func TestGetDownloadRootDir_FilesDirPreferred(t *testing.T) {
	filesDir := t.TempDir()
	rootDir := t.TempDir()
	cfg := &config.Config{
		Server: config.Server{
			FilesDir:        filesDir,
			DownloadRootDir: rootDir,
		},
	}
	m := NewManager(cfg)

	got := m.GetDownloadRootDir()
	if got != filesDir {
		t.Fatalf("expected FilesDir %q, got %q", filesDir, got)
	}
}

func TestGetDownloadRootDir_NilConfig(t *testing.T) {
	// When FilesDir and DownloadRootDir are both empty, GetDownloadRootDir
	// falls back through FileRoot() to Downloader.Filesystem.RootDir.
	// We set that to ensure a non-empty return.
	rootDir := t.TempDir()
	cfg := &config.Config{
		Server: config.Server{
			WorkDir: t.TempDir(),
		},
		Downloader: config.Downloader{
			Filesystem: config.DcFilesystem{
				RootDir: rootDir,
			},
		},
	}
	m := NewManager(cfg)
	got := m.GetDownloadRootDir()
	if got == "" {
		t.Fatal("expected non-empty download root dir")
	}
	if got != rootDir {
		t.Fatalf("expected %q, got %q", rootDir, got)
	}
}

// =============================================================================
// GetActiveDownloads
// =============================================================================

func TestGetActiveDownloads_Empty(t *testing.T) {
	m := &Manager{
		subscribers:    make(map[<-chan core.Event]chan core.Event),
		downloadingObj: sync.Map{},
		urlRegistry:    NewURLStateRegistry(),
	}
	actives := m.GetActiveDownloads()
	if len(actives) != 0 {
		t.Fatalf("expected 0 active downloads, got %d", len(actives))
	}
}

func TestGetActiveDownloads_WithObject(t *testing.T) {
	m := &Manager{
		subscribers:    make(map[<-chan core.Event]chan core.Event),
		downloadingObj: sync.Map{},
		urlRegistry:    NewURLStateRegistry(),
	}
	obj := &model.DownloadObject{
		TaskID:   "test-task",
		URL:      "http://example.com/file1",
		Metadata: map[string]string{"title": "File One"},
	}
	obj.SetProgress(50)
	obj.SetStatus(model.StatusDownloading)
	m.downloadingObj.Store(obj.URL, obj)

	actives := m.GetActiveDownloads()
	if len(actives) != 1 {
		t.Fatalf("expected 1 active download, got %d", len(actives))
	}
	entry := actives[0]
	for _, key := range []string{"task_id", "url", "title"} {
		if _, ok := entry[key]; !ok {
			t.Fatalf("expected key %q in entry", key)
		}
	}
	if entry["task_id"] != "test-task" {
		t.Fatalf("expected task_id 'test-task', got %v", entry["task_id"])
	}
	if entry["url"] != "http://example.com/file1" {
		t.Fatalf("expected url 'http://example.com/file1', got %v", entry["url"])
	}
	if entry["title"] != "File One" {
		t.Fatalf("expected title 'File One', got %v", entry["title"])
	}
	if entry["progress"] != 50 {
		t.Fatalf("expected progress 50, got %v", entry["progress"])
	}
}

func TestGetActiveDownloads_MultipleObjects(t *testing.T) {
	m := &Manager{
		subscribers:    make(map[<-chan core.Event]chan core.Event),
		downloadingObj: sync.Map{},
		urlRegistry:    NewURLStateRegistry(),
	}
	for i := range 3 {
		obj := &model.DownloadObject{
			TaskID:   "t1",
			URL:      "http://example.com/f" + string(rune('0'+i)),
			Metadata: map[string]string{"title": "File"},
		}
		obj.SetStatus(model.StatusDownloading)
		m.downloadingObj.Store(obj.URL, obj)
	}

	actives := m.GetActiveDownloads()
	if len(actives) != 3 {
		t.Fatalf("expected 3 active downloads, got %d", len(actives))
	}
}

// =============================================================================
// GetTaskSummaries
// =============================================================================

func TestGetTaskSummaries_Empty(t *testing.T) {
	cfg := &config.Config{}
	m := NewManager(cfg)
	summaries := m.GetTaskSummaries()
	if len(summaries) != 0 {
		t.Fatalf("expected 0 summaries, got %d", len(summaries))
	}
}

func TestGetTaskSummaries_WithTasks(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{ID: "task-a", Type: "mock"},
			{ID: "task-b", Type: "mock"},
		},
	}
	m := NewManager(cfg)
	m.tasks.Store("task-a", &mockTask{
		id:  "task-a",
		typ: "mock",
		objs: []*model.DownloadObject{
			{TaskID: "task-a", URL: "u1"},
			{TaskID: "task-a", URL: "u2"},
		},
	})
	m.tasks.Store("task-b", &mockTask{
		id:  "task-b",
		typ: "mock",
		objs: []*model.DownloadObject{
			{TaskID: "task-b", URL: "u3"},
		},
	})

	summaries := m.GetTaskSummaries()
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
	// Results are sorted by ID
	if summaries[0]["id"] != "task-a" {
		t.Fatalf("expected first summary id 'task-a', got %v", summaries[0]["id"])
	}
	if summaries[1]["id"] != "task-b" {
		t.Fatalf("expected second summary id 'task-b', got %v", summaries[1]["id"])
	}
	if summaries[0]["type"] != "mock" {
		t.Fatalf("expected type 'mock', got %v", summaries[0]["type"])
	}
	if summaries[0]["total"] != int64(2) {
		t.Fatalf("expected total 2, got %v", summaries[0]["total"])
	}
	if summaries[1]["total"] != int64(1) {
		t.Fatalf("expected total 1, got %v", summaries[1]["total"])
	}
}

func TestGetTaskSummaries_SkipsUnloadedTasks(t *testing.T) {
	// Config lists a task, but the Manager hasn't loaded it yet.
	cfg := &config.Config{
		Tasks: []config.Task{
			{ID: "task-only-in-config", Type: "mock"},
		},
	}
	m := NewManager(cfg)
	// Deliberately NOT storing the task in m.tasks.

	summaries := m.GetTaskSummaries()
	if len(summaries) != 0 {
		t.Fatalf("expected 0 summaries when task not loaded, got %d", len(summaries))
	}
}

// =============================================================================
// GetTaskDetails
// =============================================================================

func TestGetTaskDetails_Success(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{ID: "task-d", Type: "mock", SaveDir: "/tmp/save"},
		},
	}
	m := NewManager(cfg)
	m.tasks.Store("task-d", &mockTask{
		id:  "task-d",
		typ: "mock",
		objs: []*model.DownloadObject{
			{TaskID: "task-d", URL: "http://example.com/1"},
		},
	})

	result, err := m.GetTaskDetails("task-d", 1, 10, "", "date_asc")
	if err != nil {
		t.Fatalf("GetTaskDetails failed: %v", err)
	}
	if result["id"] != "task-d" {
		t.Fatalf("expected id 'task-d', got %v", result["id"])
	}
	if result["type"] != "mock" {
		t.Fatalf("expected type 'mock', got %v", result["type"])
	}
	if result["total"] != int64(1) {
		t.Fatalf("expected total 1, got %v", result["total"])
	}
	if result["page"] != int64(1) {
		t.Fatalf("expected page 1, got %v", result["page"])
	}
	objs, ok := result["objects"].([]*model.DownloadObject)
	if !ok || len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}
}

func TestGetTaskDetails_TaskNotFound(t *testing.T) {
	m := NewManager(&config.Config{})
	_, err := m.GetTaskDetails("nonexistent", 1, 10, "", "")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestGetTaskDetails_NoLimit(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{ID: "task-e", Type: "mock"},
		},
	}
	m := NewManager(cfg)
	m.tasks.Store("task-e", &mockTask{
		id:  "task-e",
		typ: "mock",
		objs: []*model.DownloadObject{
			{TaskID: "task-e", URL: "http://example.com/1"},
		},
	})

	result, err := m.GetTaskDetails("task-e", 1, -1, "", "")
	if err != nil {
		t.Fatalf("GetTaskDetails failed: %v", err)
	}
	objs, ok := result["objects"].([]*model.DownloadObject)
	if !ok || len(objs) != 1 {
		t.Fatalf("expected 1 object with no limit, got %d", len(objs))
	}
	if result["limit"] != int64(1) {
		t.Fatalf("expected limit adjusted to 1 (== total), got %v", result["limit"])
	}
}

func TestGetTaskDetails_WithSearch(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{ID: "task-search", Type: "mock"},
		},
	}
	m := NewManager(cfg)
	m.tasks.Store("task-search", &mockTask{
		id:  "task-search",
		typ: "mock",
		objs: []*model.DownloadObject{
			{TaskID: "task-search", URL: "http://example.com/alpha"},
			{TaskID: "task-search", URL: "http://example.com/beta"},
		},
	})

	// Search for "alpha" 鈥?the built-in search filters by URL match
	result, err := m.GetTaskDetails("task-search", 1, 10, "alpha", "")
	if err != nil {
		t.Fatalf("GetTaskDetails with search failed: %v", err)
	}
	// With GetAllObjects (no real storage), Search is applied via ApplyQueryToObjects
	// which matches the URL against the Search field.
	_ = result
}

// =============================================================================
// CancelTask / CancelTasks
// =============================================================================

func TestCancelTask_Success(t *testing.T) {
	obj := &model.DownloadObject{
		TaskID: "cancel-me",
		URL:    "http://example.com/to-cancel",
	}
	obj.SetStatus(model.StatusPending)

	m := NewManager(&config.Config{})
	m.tasks.Store("cancel-me", &mockTask{
		id:   "cancel-me",
		typ:  "mock",
		objs: []*model.DownloadObject{obj},
	})

	err := m.CancelTask("cancel-me")
	if err != nil {
		t.Fatalf("CancelTask failed: %v", err)
	}
}

func TestCancelTask_TaskNotFound(t *testing.T) {
	m := NewManager(&config.Config{})
	err := m.CancelTask("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestCancelTask_WithCompletedObject(t *testing.T) {
	obj := &model.DownloadObject{
		TaskID: "task-completed",
		URL:    "http://example.com/done",
	}
	obj.SetStatus(model.StatusCompleted)

	m := NewManager(&config.Config{})
	m.tasks.Store("task-completed", &mockTask{
		id:   "task-completed",
		typ:  "mock",
		objs: []*model.DownloadObject{obj},
	})

	// CancelTask should skip completed objects without error
	err := m.CancelTask("task-completed")
	if err != nil {
		t.Fatalf("CancelTask failed: %v", err)
	}
	// Verify the completed object wasn't changed
	if obj.GetStatus() != model.StatusCompleted {
		t.Fatalf("expected status to remain 'completed', got %q", obj.GetStatus())
	}
}

func TestCancelTasks_Mixed(t *testing.T) {
	m := NewManager(&config.Config{})
	m.tasks.Store("task-a", &mockTask{
		id:  "task-a",
		typ: "mock",
		objs: []*model.DownloadObject{
			{TaskID: "task-a", URL: "http://example.com/a1"},
		},
	})
	m.tasks.Store("task-b", &mockTask{
		id:  "task-b",
		typ: "mock",
		objs: []*model.DownloadObject{
			{TaskID: "task-b", URL: "http://example.com/b1"},
		},
	})
	// Ensure the objects are pending
	objA, _ := m.getTaskObject(getTaskFromMgr(t, m, "task-a"), "http://example.com/a1")
	objA.SetStatus(model.StatusPending)

	result := m.CancelTasks([]string{"task-a", "task-b", "nonexistent-task"})
	if result["task-a"] != "ok" {
		t.Fatalf("expected ok for task-a, got %q", result["task-a"])
	}
	if result["task-b"] != "ok" {
		t.Fatalf("expected ok for task-b, got %q", result["task-b"])
	}
	if result["nonexistent-task"] == "ok" || result["nonexistent-task"] == "" {
		t.Fatalf("expected error for nonexistent-task, got %q", result["nonexistent-task"])
	}
}

// getTaskFromMgr is a test helper that fetches a registered task by ID.
func getTaskFromMgr(t *testing.T, mgr *Manager, id string) core.Task {
	t.Helper()
	tsk, ok := mgr.getTask(id)
	if !ok {
		t.Fatalf("task %q not registered in manager", id)
	}
	return tsk
}

// =============================================================================
// CancelObject
// =============================================================================

func TestCancelObject_Success(t *testing.T) {
	obj := &model.DownloadObject{
		TaskID: "task-co",
		URL:    "http://example.com/cancel-obj",
	}
	obj.SetStatus(model.StatusPending)

	m := NewManager(&config.Config{})
	m.tasks.Store("task-co", &mockTask{
		id:   "task-co",
		typ:  "mock",
		objs: []*model.DownloadObject{obj},
	})

	err := m.CancelObject("task-co", "http://example.com/cancel-obj")
	if err != nil {
		t.Fatalf("CancelObject failed: %v", err)
	}
}

func TestCancelObject_TaskNotFound(t *testing.T) {
	m := NewManager(&config.Config{})
	err := m.CancelObject("nonexistent", "http://example.com/u")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestCancelObject_AlreadyCompleted(t *testing.T) {
	obj := &model.DownloadObject{
		TaskID: "task-co2",
		URL:    "http://example.com/done",
	}
	obj.SetStatus(model.StatusCompleted)

	m := NewManager(&config.Config{})
	m.tasks.Store("task-co2", &mockTask{
		id:   "task-co2",
		typ:  "mock",
		objs: []*model.DownloadObject{obj},
	})

	err := m.CancelObject("task-co2", "http://example.com/done")
	if err == nil {
		t.Fatal("expected error for already completed object")
	}
}

// =============================================================================
// UndoCancelObject
// =============================================================================

func TestUndoCancelObject_Success(t *testing.T) {
	obj := &model.DownloadObject{
		TaskID: "task-undo",
		URL:    "http://example.com/undo",
	}
	obj.SetStatus(model.StatusCancelled)

	m := NewManager(&config.Config{})
	m.tasks.Store("task-undo", &mockTask{
		id:   "task-undo",
		typ:  "mock",
		objs: []*model.DownloadObject{obj},
	})

	err := m.UndoCancelObject("task-undo", "http://example.com/undo")
	if err != nil {
		t.Fatalf("UndoCancelObject failed: %v", err)
	}
	// mockTask.UpdateStatus is a no-op, so the object status won't change
	// here. We test the Manager orchestration path succeeds.
}

func TestUndoCancelObject_TaskNotFound(t *testing.T) {
	m := NewManager(&config.Config{})
	err := m.UndoCancelObject("nonexistent", "http://example.com/u")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestUndoCancelObject_NotCancelled(t *testing.T) {
	obj := &model.DownloadObject{
		TaskID: "task-undo2",
		URL:    "http://example.com/pending",
	}
	obj.SetStatus(model.StatusPending)

	m := NewManager(&config.Config{})
	m.tasks.Store("task-undo2", &mockTask{
		id:   "task-undo2",
		typ:  "mock",
		objs: []*model.DownloadObject{obj},
	})

	err := m.UndoCancelObject("task-undo2", "http://example.com/pending")
	if err == nil {
		t.Fatal("expected error for non-cancelled object")
	}
}

func TestUndoCancelObject_ObjectNotFound(t *testing.T) {
	obj := &model.DownloadObject{
		TaskID: "task-undo3",
		URL:    "http://example.com/exists",
	}
	obj.SetStatus(model.StatusCancelled)

	m := NewManager(&config.Config{})
	m.tasks.Store("task-undo3", &mockTask{
		id:   "task-undo3",
		typ:  "mock",
		objs: []*model.DownloadObject{obj},
	})

	// Request a URL that is not among the task's objects
	err := m.UndoCancelObject("task-undo3", "http://example.com/does-not-exist")
	if err == nil {
		t.Fatal("expected error for non-existent object URL")
	}
}

// =============================================================================
// ReorderObject
// =============================================================================

// reorderableTask extends mockTask with SetObjectIndex support.
type reorderableTask struct {
	mockTask
	setIndexedURL string
	setIndex      int
}

func (t *reorderableTask) SetObjectIndex(url string, newIndex int) error {
	t.setIndexedURL = url
	t.setIndex = newIndex
	return nil
}

func TestReorderObject_Success(t *testing.T) {
	task := &reorderableTask{
		mockTask: mockTask{
			id:  "task-reorder",
			typ: "mock",
		},
	}
	m := NewManager(&config.Config{})
	m.tasks.Store("task-reorder", task)

	err := m.ReorderObject("task-reorder", "http://example.com/obj", 0)
	if err != nil {
		t.Fatalf("ReorderObject failed: %v", err)
	}
	if task.setIndexedURL != "http://example.com/obj" {
		t.Fatalf("expected URL 'http://example.com/obj', got %q", task.setIndexedURL)
	}
	if task.setIndex != 0 {
		t.Fatalf("expected index 0, got %d", task.setIndex)
	}
}

func TestReorderObject_TaskNotFound(t *testing.T) {
	m := NewManager(&config.Config{})
	err := m.ReorderObject("nonexistent", "http://example.com/obj", 0)
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestReorderObject_NoSupport(t *testing.T) {
	m := NewManager(&config.Config{})
	m.tasks.Store("no-reorder", &mockTask{
		id:  "no-reorder",
		typ: "mock",
	})
	err := m.ReorderObject("no-reorder", "http://example.com/obj", 0)
	if err == nil {
		t.Fatal("expected error for task that does not support reordering")
	}
}

// =============================================================================
// GetConfig
// =============================================================================

func TestGetConfig_ReturnsNonNil(t *testing.T) {
	cfg := &config.Config{
		Server: config.Server{
			WorkDir: t.TempDir(),
		},
		Tasks: []config.Task{
			{ID: "t1", Type: "mock"},
		},
	}
	m := NewManager(cfg)
	got := m.GetConfig()
	if got == nil {
		t.Fatal("GetConfig returned nil")
	}
	if len(got.Tasks) != 1 || got.Tasks[0].ID != "t1" {
		t.Fatalf("expected 1 task with ID 't1', got %+v", got.Tasks)
	}
}
