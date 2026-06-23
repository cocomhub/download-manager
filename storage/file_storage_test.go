// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// TestFileStorage_RoundTrip verifies Update/Get round-trip preserves all fields.
func TestFileStorage_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_roundtrip.json")
	fs, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	obj := &model.DownloadObject{
		TaskID:   "task1",
		URL:      "http://example.com/file1",
		SavePath: "/tmp/file1.bin",
		Status:   model.StatusPending,
		Progress: 0,
		Metadata: map[string]string{"title": "Test File"},
	}

	if err := fs.Update(obj); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, err := fs.Get(obj.URL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.URL != obj.URL {
		t.Fatalf("URL = %q, want %q", got.URL, obj.URL)
	}
	if got.TaskID != obj.TaskID {
		t.Fatalf("TaskID = %q, want %q", got.TaskID, obj.TaskID)
	}
	if got.Status != obj.Status {
		t.Fatalf("Status = %q, want %q", got.Status, obj.Status)
	}
	if got.Metadata[model.MetadataKeyTitle] != "Test File" {
		t.Fatalf("Metadata title = %q, want %q", got.Metadata[model.MetadataKeyTitle], "Test File")
	}
}

// TestFileStorage_UpdateOverwrites verifies updating an existing URL overwrites it.
func TestFileStorage_UpdateOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_overwrite.json")
	fs, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	obj1 := &model.DownloadObject{URL: "http://example.com/file", TaskID: "task1", Status: model.StatusPending}
	if err := fs.Update(obj1); err != nil {
		t.Fatalf("first Update failed: %v", err)
	}

	obj2 := &model.DownloadObject{URL: "http://example.com/file", TaskID: "task2", Status: model.StatusCompleted}
	if err := fs.Update(obj2); err != nil {
		t.Fatalf("second Update failed: %v", err)
	}

	got, _ := fs.Get("http://example.com/file")
	if got.TaskID != "task2" {
		t.Fatalf("TaskID after overwrite = %q, want %q", got.TaskID, "task2")
	}
}

// TestFileStorage_GetNilForMissing verifies Get returns nil for non-existent object.
func TestFileStorage_GetNilForMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_missing.json")
	fs, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	got, err := fs.Get("http://example.com/nonexistent")
	if err != nil {
		t.Fatalf("Get for missing returned error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for missing object")
	}
}

// TestFileStorage_DeleteRemovesObject verifies Delete removes the object.
func TestFileStorage_DeleteRemovesObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_delete.json")
	fs, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	obj := &model.DownloadObject{URL: "http://example.com/file", Status: model.StatusPending}
	fs.Update(obj)
	if err := fs.Delete(obj.URL); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	got, _ := fs.Get(obj.URL)
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

// TestFileStorage_ForceFlush verifies ForceFlush writes data to disk and can be reloaded.
func TestFileStorage_ForceFlush(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_flush.json")
	fs, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	obj := &model.DownloadObject{URL: "http://example.com/file", Status: model.StatusCompleted}
	fs.Update(obj)
	if err := fs.ForceFlush(); err != nil {
		t.Fatalf("ForceFlush failed: %v", err)
	}

	// Verify file exists on disk
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("flush file not found on disk")
	}

	// Reload from disk 鈥?create new FileStorage pointing to same path
	fs2, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage reload failed: %v", err)
	}

	got, err := fs2.Get(obj.URL)
	if err != nil {
		t.Fatalf("reload Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("reloaded object is nil")
	}
	if got.Status != model.StatusCompleted {
		t.Fatalf("reloaded status = %q, want %q", got.Status, model.StatusCompleted)
	}
}

// TestFileStorage_Search verifies Search with filters.
func TestFileStorage_Search(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_search.json")
	fs, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	for i := range 10 {
		status := model.StatusPending
		if i%2 == 0 {
			status = model.StatusCompleted
		}
		fs.Update(&model.DownloadObject{
			URL:    fmt.Sprintf("http://example.com/file-%d.bin", i),
			Status: status,
			TaskID: "search-task",
		})
	}

	// Search for pending objects
	results, err := fs.Search(&core.StorageQuery{
		Filter: core.StorageFilter{
			Statuses: []string{model.StatusPending},
		},
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 pending objects, got %d", len(results))
	}
}

// TestFileStorage_Count verifies Count returns correct number.
func TestFileStorage_Count(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_count.json")
	fs, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	for i := range 7 {
		fs.Update(&model.DownloadObject{
			URL:    fmt.Sprintf("http://example.com/file-%d.bin", i),
			Status: model.StatusPending,
			TaskID: "count-task",
		})
	}

	count, err := fs.Count(nil)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 7 {
		t.Fatalf("expected count 7, got %d", count)
	}
}

// TestFileStorage_Exists verifies Exists returns correct booleans.
func TestFileStorage_Exists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_exists.json")
	fs, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	fs.Update(&model.DownloadObject{URL: "http://a.com", Status: model.StatusPending})
	fs.Update(&model.DownloadObject{URL: "http://b.com", Status: model.StatusCompleted})

	result, err := fs.Exists([]string{"http://a.com", "http://b.com", "http://c.com"})
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !result["http://a.com"] {
		t.Fatal("expected a.com to exist")
	}
	if !result["http://b.com"] {
		t.Fatal("expected b.com to exist")
	}
	if result["http://c.com"] {
		t.Fatal("expected c.com to not exist")
	}
}

// TestFileStorage_LoadFromExistingFile verifies pre-existing file is loaded.
func TestFileStorage_LoadFromExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preload.json")

	// Write a pre-existing file
	content := `[{"url":"http://preload.com/file", "task_id":"preload", "status":"completed"}]`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	fs, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	obj, _ := fs.Get("http://preload.com/file")
	if obj == nil {
		t.Fatal("preloaded object not found")
	}
	if obj.TaskID != "preload" {
		t.Fatalf("TaskID = %q, want %q", obj.TaskID, "preload")
	}
}
