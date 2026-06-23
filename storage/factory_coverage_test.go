// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// newTestMemoryStorage creates a MemoryStorage pre-populated with test objects.
func newTestMemoryStorage(t *testing.T, objects map[string]*model.DownloadObject) *MemoryStorage {
	t.Helper()
	return &MemoryStorage{objects: objects}
}

// testObject creates a DownloadObject with the given URL and status.
func testObject(url, status string) *model.DownloadObject {
	return &model.DownloadObject{
		URL:    url,
		Status: status,
		TaskID: "task1",
	}
}

func TestMemoryStorage_Get(t *testing.T) {
	s := newTestMemoryStorage(t, map[string]*model.DownloadObject{
		"http://example.com/file1": testObject("http://example.com/file1", model.StatusPending),
	})

	tests := []struct {
		name    string
		id      string
		wantURL string
		wantNil bool
	}{
		{
			name:    "existing object returns the object",
			id:      "http://example.com/file1",
			wantURL: "http://example.com/file1",
			wantNil: false,
		},
		{
			name:    "missing object returns nil",
			id:      "http://example.com/nonexistent",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj, err := s.Get(tt.id)
			if err != nil {
				t.Fatalf("Get(%q) returned error: %v", tt.id, err)
			}
			if tt.wantNil {
				if obj != nil {
					t.Fatalf("Get(%q) = %v, want nil", tt.id, obj)
				}
				return
			}
			if obj == nil {
				t.Fatalf("Get(%q) = nil, want non-nil", tt.id)
			}
			if obj.URL != tt.wantURL {
				t.Errorf("Get(%q).URL = %q, want %q", tt.id, obj.URL, tt.wantURL)
			}
		})
	}
}

func TestMemoryStorage_Update(t *testing.T) {
	t.Run("new object is stored", func(t *testing.T) {
		s := &MemoryStorage{objects: make(map[string]*model.DownloadObject)}
		obj := testObject("http://example.com/new", model.StatusPending)
		if err := s.Update(obj); err != nil {
			t.Fatalf("Update returned error: %v", err)
		}
		got, err := s.Get("http://example.com/new")
		if err != nil {
			t.Fatalf("Get after Update: %v", err)
		}
		if got == nil {
			t.Fatal("Get after Update returned nil")
		}
		if got.URL != "http://example.com/new" {
			t.Errorf("URL = %q, want %q", got.URL, "http://example.com/new")
		}
	})

	t.Run("update existing object overwrites", func(t *testing.T) {
		s := newTestMemoryStorage(t, map[string]*model.DownloadObject{
			"http://example.com/existing": testObject("http://example.com/existing", model.StatusPending),
		})
		updated := testObject("http://example.com/existing", model.StatusCompleted)
		if err := s.Update(updated); err != nil {
			t.Fatalf("Update returned error: %v", err)
		}
		got, err := s.Get("http://example.com/existing")
		if err != nil {
			t.Fatalf("Get after Update: %v", err)
		}
		if got.GetStatus() != model.StatusCompleted {
			t.Errorf("status = %q, want %q", got.GetStatus(), model.StatusCompleted)
		}
	})

	t.Run("nil object does not error", func(t *testing.T) {
		s := &MemoryStorage{objects: make(map[string]*model.DownloadObject)}
		if err := s.Update(nil); err != nil {
			t.Fatalf("Update(nil) returned error: %v", err)
		}
	})
}

func TestMemoryStorage_Delete(t *testing.T) {
	t.Run("existing object is deleted", func(t *testing.T) {
		s := newTestMemoryStorage(t, map[string]*model.DownloadObject{
			"http://example.com/todelete": testObject("http://example.com/todelete", model.StatusPending),
		})
		if err := s.Delete("http://example.com/todelete"); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
		got, err := s.Get("http://example.com/todelete")
		if err != nil {
			t.Fatalf("Get after Delete: %v", err)
		}
		if got != nil {
			t.Fatal("Get after Delete should return nil")
		}
	})

	t.Run("missing object does not error", func(t *testing.T) {
		s := &MemoryStorage{objects: make(map[string]*model.DownloadObject)}
		if err := s.Delete("http://example.com/nonexistent"); err != nil {
			t.Fatalf("Delete(nonexistent) returned error: %v", err)
		}
	})
}

func TestMemoryStorage_Search(t *testing.T) {
	objects := map[string]*model.DownloadObject{
		"http://a.com": {URL: "http://a.com", TaskID: "task1", Status: model.StatusPending},
		"http://b.com": {URL: "http://b.com", TaskID: "task1", Status: model.StatusCompleted},
		"http://c.com": {URL: "http://c.com", TaskID: "task2", Status: model.StatusPending},
		"http://d.com": {URL: "http://d.com", TaskID: "task2", Status: model.StatusFailed},
	}

	tests := []struct {
		name      string
		query     *core.StorageQuery
		wantURLs  []string
		wantCount int
	}{
		{
			name:      "nil query returns all objects",
			query:     nil,
			wantCount: 4,
		},
		{
			name: "filter by status returns matching objects",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					Statuses: []string{model.StatusPending},
				},
			},
			wantURLs: []string{"http://a.com", "http://c.com"},
		},
		{
			name: "filter by task id and status",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					TaskIDs:  []string{"task1"},
					Statuses: []string{model.StatusPending},
				},
			},
			wantURLs: []string{"http://a.com"},
		},
		{
			name: "search by URL substring",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					Search: "a.com",
				},
			},
			wantURLs: []string{"http://a.com"},
		},
		{
			name: "offset and limit",
			query: &core.StorageQuery{
				Offset: 1,
				Limit:  2,
			},
			wantCount: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestMemoryStorage(t, objects)
			results, err := s.Search(tt.query)
			if err != nil {
				t.Fatalf("Search returned error: %v", err)
			}
			if tt.wantURLs != nil {
				if len(results) != len(tt.wantURLs) {
					t.Fatalf("Search returned %d results, want %d", len(results), len(tt.wantURLs))
				}
				got := make(map[string]bool, len(results))
				for _, r := range results {
					got[r.URL] = true
				}
				for _, want := range tt.wantURLs {
					if !got[want] {
						t.Errorf("result missing URL %q; got %v", want, got)
					}
				}
			} else if tt.wantCount >= 0 {
				if len(results) != tt.wantCount {
					t.Errorf("Search returned %d results, want %d", len(results), tt.wantCount)
				}
			}
		})
	}
}

func TestMemoryStorage_Count(t *testing.T) {
	objects := map[string]*model.DownloadObject{
		"http://a.com": {URL: "http://a.com", TaskID: "task1", Status: model.StatusPending},
		"http://b.com": {URL: "http://b.com", TaskID: "task1", Status: model.StatusCompleted},
		"http://c.com": {URL: "http://c.com", TaskID: "task2", Status: model.StatusPending},
	}

	tests := []struct {
		name  string
		query *core.StorageQuery
		want  int64
	}{
		{
			name:  "count all objects with nil query",
			query: nil,
			want:  3,
		},
		{
			name: "count by task id",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					TaskIDs: []string{"task1"},
				},
			},
			want: 2,
		},
		{
			name: "count by status",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					Statuses: []string{model.StatusPending},
				},
			},
			want: 2,
		},
		{
			name: "count no match returns zero",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					TaskIDs:  []string{"task1"},
					Statuses: []string{model.StatusFailed},
				},
			},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestMemoryStorage(t, objects)
			got, err := s.Count(tt.query)
			if err != nil {
				t.Fatalf("Count returned error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Count = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMemoryStorage_Exists(t *testing.T) {
	objects := map[string]*model.DownloadObject{
		"http://a.com": testObject("http://a.com", model.StatusPending),
		"http://b.com": testObject("http://b.com", model.StatusCompleted),
	}

	tests := []struct {
		name string
		ids  []string
		want map[string]bool
	}{
		{
			name: "some exist some don't",
			ids:  []string{"http://a.com", "http://c.com", "http://b.com"},
			want: map[string]bool{
				"http://a.com": true,
				"http://c.com": false,
				"http://b.com": true,
			},
		},
		{
			name: "empty ids returns empty map",
			ids:  []string{},
			want: map[string]bool{},
		},
		{
			name: "none exist",
			ids:  []string{"http://x.com", "http://y.com"},
			want: map[string]bool{
				"http://x.com": false,
				"http://y.com": false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestMemoryStorage(t, objects)
			got, err := s.Exists(tt.ids)
			if err != nil {
				t.Fatalf("Exists returned error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("Exists returned %d results, want %d", len(got), len(tt.want))
			}
			for id, exists := range tt.want {
				if got[id] != exists {
					t.Errorf("Exists(%q) = %v, want %v", id, got[id], exists)
				}
			}
		})
	}
}

func TestMemoryStorage_ConcurrentSafety(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	s := &MemoryStorage{objects: make(map[string]*model.DownloadObject)}

	// Pre-populate some objects.
	for i := range 10 {
		url := "http://example.com/file-" + itoa(i) + ".zip"
		_ = s.Update(&model.DownloadObject{URL: url, TaskID: "task1", Status: model.StatusPending})
	}

	var wg sync.WaitGroup

	// Concurrent readers: Get and Search.
	wg.Go(func() {
		for ctx.Err() == nil {
			_, _ = s.Get("http://example.com/file-0.zip")
			_, _ = s.Search(&core.StorageQuery{
				Filter: core.StorageFilter{
					TaskIDs: []string{"task1"},
				},
			})
		}
	})

	// Concurrent writers: Update, Delete, Count, and Exists.
	wg.Go(func() {
		for ctx.Err() == nil {
			_ = s.Update(&model.DownloadObject{URL: "http://example.com/file-0.zip", Status: model.StatusCompleted})
			_ = s.Delete("http://example.com/file-1.zip")
			_ = s.Update(&model.DownloadObject{URL: "http://example.com/file-1.zip", Status: model.StatusPending})
			_, _ = s.Count(&core.StorageQuery{
				Filter: core.StorageFilter{
					Statuses: []string{model.StatusPending},
				},
			})
			_, _ = s.Exists([]string{"http://example.com/file-0.zip", "http://example.com/file-1.zip"})
		}
	})

	wg.Wait()
}

// itoa is a minimal int-to-string helper for test data generation.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
