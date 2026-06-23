// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// TestFileStorage_Search_MultiFilter verifies Search with combined filters
// (status + search + metadata).
func TestFileStorage_Search_MultiFilter(t *testing.T) {
	fs := newTestFileStorage(t)

	objects := []*model.DownloadObject{
		{URL: "http://a.com/1", TaskID: "t1", Status: model.StatusCompleted, Metadata: map[string]string{"title": "Alpha"}},
		{URL: "http://b.com/2", TaskID: "t1", Status: model.StatusPending, Metadata: map[string]string{"title": "Beta"}},
		{URL: "http://c.com/3", TaskID: "t2", Status: model.StatusCompleted, Metadata: map[string]string{"title": "Charlie"}},
		{URL: "http://d.com/4", TaskID: "t2", Status: model.StatusFailed, Metadata: map[string]string{"title": "Delta"}},
	}
	for _, obj := range objects {
		fs.Update(obj)
	}

	tests := []struct {
		name  string
		query *core.StorageQuery
		want  int
		desc  string
	}{
		{
			name: "status+task",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					TaskIDs:  []string{"t1"},
					Statuses: []string{model.StatusCompleted},
				},
			},
			want: 1,
			desc: "completed + task t1 = 1 result",
		},
		{
			name: "status+task+search",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					TaskIDs:  []string{"t2"},
					Statuses: []string{model.StatusCompleted, model.StatusFailed},
					Search:   "Charlie",
				},
			},
			want: 1,
			desc: "t2 + completed/failed + search Charlie = 1 result",
		},
		{
			name: "search url partial",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					Search: "b.com",
				},
			},
			want: 1,
			desc: "search URL substring = 1 result",
		},
		{
			name: "metadata key match via title search",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					Statuses: []string{model.StatusCompleted},
					Search:   "Alpha",
				},
			},
			want: 1,
			desc: "completed + search Alpha title = 1 result",
		},
		{
			name:  "no match",
			query: &core.StorageQuery{Filter: core.StorageFilter{Statuses: []string{model.StatusCancelled}}},
			want:  0,
			desc:  "cancelled status no match = 0",
		},
		{
			name: "all with no filter",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{},
			},
			want: 4,
			desc: "no filter = all 4 objects",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := fs.Search(tt.query)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}
			if len(results) != tt.want {
				t.Errorf("Search %s: expected %d results, got %d", tt.desc, tt.want, len(results))
			}
		})
	}
}

// TestFileStorage_Search_Pagination verifies Search with offset and limit.
func TestFileStorage_Search_Pagination(t *testing.T) {
	fs := newTestFileStorage(t)

	for i := range 20 {
		fs.Update(&model.DownloadObject{
			URL:    fmt.Sprintf("http://example.com/file-%d.bin", i),
			TaskID: "pagination-task",
		})
	}

	tests := []struct {
		name  string
		query *core.StorageQuery
		want  int
		desc  string
	}{
		{
			name: "limit 5",
			query: &core.StorageQuery{
				Limit:  5,
				Offset: 0,
			},
			want: 5,
		},
		{
			name: "offset 15 limit 10",
			query: &core.StorageQuery{
				Limit:  10,
				Offset: 15,
			},
			want: 5, // Only 5 remaining
		},
		{
			name: "offset beyond total",
			query: &core.StorageQuery{
				Limit:  5,
				Offset: 100,
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := fs.Search(tt.query)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}
			if len(results) != tt.want {
				t.Errorf("%s: expected %d results, got %d", tt.desc, tt.want, len(results))
			}
		})
	}
}

// TestFileStorage_ConcurrentUpdate tests that concurrent Updates don't race.
func TestFileStorage_ConcurrentUpdate(t *testing.T) {
	fs := newTestFileStorage(t)

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			obj := &model.DownloadObject{
				URL:    fmt.Sprintf("http://example.com/concurrent-%d", n),
				TaskID: "concurrent-task",
			}
			if err := fs.Update(obj); err != nil {
				t.Errorf("Update failed: %v", err)
			}
		}(i)
	}
	wg.Wait()

	// Verify all 50 objects are in storage
	count, err := fs.Count(nil)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 50 {
		t.Errorf("expected 50 objects after concurrent Update, got %d", count)
	}
}

// TestFileStorage_FlushAndRecover verifies that data survives flush -> new instance.
func TestFileStorage_FlushAndRecover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recover.json")

	fs, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	// Insert some data
	for i := range 5 {
		fs.Update(&model.DownloadObject{
			URL:    fmt.Sprintf("http://recover.com/file-%d.bin", i),
			Status: model.StatusCompleted,
		})
	}
	fs.ForceFlush()

	// Create new instance from the same file
	fs2, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage reload failed: %v", err)
	}

	for i := range 5 {
		url := fmt.Sprintf("http://recover.com/file-%d.bin", i)
		obj, _ := fs2.Get(url)
		if obj == nil {
			t.Errorf("recovered object %s not found", url)
		} else if obj.Status != model.StatusCompleted {
			t.Errorf("recovered object %s status = %s, want completed", url, obj.Status)
		}
	}
}

// TestFileStorage_CorruptedFile verifies behavior with corrupted JSON file.
func TestFileStorage_CorruptedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.json")

	if err := os.WriteFile(path, []byte("{invalid json}"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err := NewFileStorage(map[string]string{"path": path})
	if err == nil {
		t.Fatal("expected error for corrupted JSON file, got nil")
	}
}

// TestFileStorage_Search_Clamping verifies Search clamps negative limit/offset.
func TestFileStorage_Search_Clamping(t *testing.T) {
	fs := newTestFileStorage(t)

	for i := range 10 {
		fs.Update(&model.DownloadObject{
			URL: fmt.Sprintf("http://clamp.com/file-%d.bin", i),
		})
	}

	tests := []struct {
		name  string
		query *core.StorageQuery
		want  int
	}{
		{"negative limit", &core.StorageQuery{Limit: -1, Offset: 0}, 10},
		{"negative offset", &core.StorageQuery{Limit: 5, Offset: -1}, 5},
		{"zero limit", &core.StorageQuery{Limit: 0, Offset: 0}, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := fs.Search(tt.query)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}
			if len(results) != tt.want {
				t.Errorf("expected %d results, got %d", tt.want, len(results))
			}
		})
	}
}

// TestFileStorage_Search_Sort verifies Search with sort options.
func TestFileStorage_Search_Sort(t *testing.T) {
	fs := newTestFileStorage(t)

	for i := range 5 {
		fs.Update(&model.DownloadObject{
			URL:    fmt.Sprintf("http://sort.com/file-%d.bin", i),
			TaskID: "sort-task",
			Metadata: map[string]string{
				"title": fmt.Sprintf("Title %d", 4-i), // Reverse order
			},
		})
	}

	results, err := fs.Search(&core.StorageQuery{
		Sort: []core.StorageSort{{Field: "name", Desc: false}},
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
}

// TestFileStorage_Exists_Multiple verifies Exists with multiple IDs.
func TestFileStorage_Exists_Multiple(t *testing.T) {
	fs := newTestFileStorage(t)

	for i := range 5 {
		fs.Update(&model.DownloadObject{
			URL: fmt.Sprintf("http://exists.com/file-%d.bin", i),
		})
	}

	ids := make([]string, 7)
	for i := range 7 {
		ids[i] = fmt.Sprintf("http://exists.com/file-%d.bin", i)
	}

	result, err := fs.Exists(ids)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}

	for i := range 5 {
		url := fmt.Sprintf("http://exists.com/file-%d.bin", i)
		if !result[url] {
			t.Errorf("expected %s to exist", url)
		}
	}
	for i := 5; i < 7; i++ {
		url := fmt.Sprintf("http://exists.com/file-%d.bin", i)
		if result[url] {
			t.Errorf("expected %s to not exist", url)
		}
	}
}

// TestFileStorage_UpdateAfterForceFlush verifies that new data written after
// a ForceFlush is also persisted.
func TestFileStorage_UpdateAfterForceFlush(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "append.json")

	fs, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	fs.Update(&model.DownloadObject{URL: "http://a.com", Status: model.StatusPending})
	fs.ForceFlush()

	fs.Update(&model.DownloadObject{URL: "http://b.com", Status: model.StatusCompleted})
	fs.ForceFlush()

	// Recover
	fs2, _ := NewFileStorage(map[string]string{"path": path})
	objA, _ := fs2.Get("http://a.com")
	objB, _ := fs2.Get("http://b.com")
	if objA == nil || objB == nil {
		t.Error("expected both objects to survive flush")
	}
}

// Timeout support for FileStorage test
func TestFileStorage_LoadTimeout_Defaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "timeout_default.json")
	fs, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	if fs.saveInterval != time.Second {
		t.Errorf("default saveInterval = %v, want 1s", fs.saveInterval)
	}
}

// newTestFileStorage creates a FileStorage backed by a temp file for each test.
func newTestFileStorage(t *testing.T) *FileStorage {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test_storage.json")
	fs, err := NewFileStorage(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	return fs
}
