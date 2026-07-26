// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"log/slog"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
)

// =============================================================================
// GetObjectByTypeAndID
// =============================================================================

func TestGetObjectByTypeAndID_Found(t *testing.T) {
	st, err := storage.NewStorage("memory", nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	obj := &model.DownloadObject{TaskID: "t1", URL: "http://example.com/1"}
	obj.SetID(42)
	if err := st.Update(obj); err != nil {
		t.Fatalf("Update: %v", err)
	}

	m := NewManager(&config.Config{})
	m.tasks.Store("t1", &mockTaskWithStorage{id: "t1", typ: "type-a", st: st})

	got, err := m.GetObjectByTypeAndID("type-a", 42)
	if err != nil {
		t.Fatalf("GetObjectByTypeAndID failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil object")
	}
	if got.GetID() != 42 {
		t.Fatalf("expected ID=42, got %d", got.GetID())
	}
}

func TestGetObjectByTypeAndID_NotFoundID(t *testing.T) {
	st, err := storage.NewStorage("memory", nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	obj := &model.DownloadObject{TaskID: "t1", URL: "http://example.com/1"}
	obj.SetID(42)
	if err := st.Update(obj); err != nil {
		t.Fatalf("Update: %v", err)
	}

	m := NewManager(&config.Config{})
	m.tasks.Store("t1", &mockTaskWithStorage{id: "t1", typ: "type-a", st: st})

	got, err := m.GetObjectByTypeAndID("type-a", 999)
	if err != nil {
		t.Fatalf("GetObjectByTypeAndID failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for unknown ID, got object with ID=%d", got.GetID())
	}
}

func TestGetObjectByTypeAndID_NotFoundType(t *testing.T) {
	m := NewManager(&config.Config{})
	_, err := m.GetObjectByTypeAndID("nonexistent", 1)
	if err == nil {
		t.Fatal("expected error for nonexistent task type")
	}
}

func TestGetObjectByTypeAndID_NilStorage(t *testing.T) {
	m := NewManager(&config.Config{})
	m.tasks.Store("t1", &mockTask{id: "t1", typ: "type-a"}) // mockTask returns nil Storage

	got, err := m.GetObjectByTypeAndID("type-a", 1)
	if err != nil {
		t.Fatalf("GetObjectByTypeAndID failed: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil when storage is nil")
	}
}

// =============================================================================
// GetCollectionByID
// =============================================================================

func TestGetCollectionByID_Success(t *testing.T) {
	st, err := storage.NewStorage("memory", nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	obj1 := &model.DownloadObject{TaskID: "t1", URL: "http://example.com/1"}
	obj1.SetID(1)
	obj1.Lock()
	obj1.Metadata = map[string]string{"collection_id": "coll-1", "collection_title": "A"}
	obj1.Unlock()
	if err := st.Update(obj1); err != nil {
		t.Fatalf("Update obj1: %v", err)
	}

	obj2 := &model.DownloadObject{TaskID: "t1", URL: "http://example.com/2"}
	obj2.SetID(2)
	obj2.Lock()
	obj2.Metadata = map[string]string{"collection_id": "coll-1", "collection_title": "B"}
	obj2.Unlock()
	if err := st.Update(obj2); err != nil {
		t.Fatalf("Update obj2: %v", err)
	}

	// Different collection, should not be returned
	obj3 := &model.DownloadObject{TaskID: "t1", URL: "http://example.com/3"}
	obj3.SetID(3)
	obj3.Lock()
	obj3.Metadata = map[string]string{"collection_id": "coll-2", "collection_title": "C"}
	obj3.Unlock()
	if err := st.Update(obj3); err != nil {
		t.Fatalf("Update obj3: %v", err)
	}

	m := NewManager(&config.Config{})
	m.tasks.Store("t1", &mockTaskWithStorage{id: "t1", typ: "type-a", st: st})

	objects, err := m.GetCollectionByID("type-a", 1)
	if err != nil {
		t.Fatalf("GetCollectionByID failed: %v", err)
	}
	if len(objects) != 2 {
		t.Fatalf("expected 2 objects in collection, got %d", len(objects))
	}
	// Should be sorted by collection_title
	if objects[0].URL != "http://example.com/1" || objects[1].URL != "http://example.com/2" {
		t.Errorf("expected order: u1, u2, got %s, %s", objects[0].URL, objects[1].URL)
	}
}

func TestGetCollectionByID_NoCollection(t *testing.T) {
	st, err := storage.NewStorage("memory", nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	obj := &model.DownloadObject{TaskID: "t1", URL: "http://example.com/1"}
	obj.SetID(1)
	obj.Lock()
	obj.Metadata = map[string]string{"title": "no collection"}
	obj.Unlock()
	if err := st.Update(obj); err != nil {
		t.Fatalf("Update: %v", err)
	}

	m := NewManager(&config.Config{})
	m.tasks.Store("t1", &mockTaskWithStorage{id: "t1", typ: "type-a", st: st})

	objects, err := m.GetCollectionByID("type-a", 1)
	if err != nil {
		t.Fatalf("GetCollectionByID failed: %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("expected empty collection, got %d objects", len(objects))
	}
}

func TestGetCollectionByID_NotFoundID(t *testing.T) {
	st, err := storage.NewStorage("memory", nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	m := NewManager(&config.Config{})
	m.tasks.Store("t1", &mockTaskWithStorage{id: "t1", typ: "type-a", st: st})

	objects, err := m.GetCollectionByID("type-a", 999)
	if err != nil {
		t.Fatalf("GetCollectionByID failed: %v", err)
	}
	if objects != nil {
		t.Fatal("expected nil when object not found")
	}
}

func TestGetCollectionByID_NotFoundType(t *testing.T) {
	m := NewManager(&config.Config{})
	_, err := m.GetCollectionByID("nonexistent", 1)
	if err == nil {
		t.Fatal("expected error for nonexistent task type")
	}
}

// mockTaskWithStorage is a mock task that returns a non-nil Storage.
type mockTaskWithStorage struct {
	id  string
	typ string
	st  core.Storage
}

func (m *mockTaskWithStorage) ID() string                            { return m.id }
func (m *mockTaskWithStorage) Type() string                          { return m.typ }
func (m *mockTaskWithStorage) Logger() *slog.Logger                  { return slog.Default() }
func (m *mockTaskWithStorage) Storage() core.Storage                 { return m.st }
func (m *mockTaskWithStorage) SetDownloader(core.Downloader)         {}
func (m *mockTaskWithStorage) GetDownloadHeaders() map[string]string { return nil }
func (m *mockTaskWithStorage) GetDownloadObjects() ([]*model.DownloadObject, error) {
	return nil, nil
}
func (m *mockTaskWithStorage) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	return nil
}
func (m *mockTaskWithStorage) Concurrency() int             { return 1 }
func (m *mockTaskWithStorage) SetConcurrency(int) error     { return nil }
func (m *mockTaskWithStorage) RefreshInterval() int         { return 0 }
func (m *mockTaskWithStorage) SetRefreshInterval(int) error { return nil }
func (m *mockTaskWithStorage) Start() error                 { return nil }
func (m *mockTaskWithStorage) ResolveObject(_ context.Context, _ *model.DownloadObject) error {
	return nil
}
func (m *mockTaskWithStorage) Close() error                                    { return nil }
func (m *mockTaskWithStorage) GetAllObjects(lock bool) []*model.DownloadObject { return nil }
