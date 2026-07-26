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

// mockStandardizer implements core.Task + core.Standardizer for testing.
type mockStandardizer struct {
	mockTask
	st        core.Storage
	callCount int
}

func (m *mockStandardizer) Storage() core.Storage { return m.st }

func (m *mockStandardizer) Standardize(obj *model.DownloadObject) (bool, error) {
	m.callCount++
	obj.SetID(42)
	return true, nil
}

// =============================================================================
// UniqueTaskTypes
// =============================================================================

func TestUniqueTaskTypes_Empty(t *testing.T) {
	m := NewManager(&config.Config{})
	types := m.UniqueTaskTypes()
	if len(types) != 0 {
		t.Fatalf("expected 0 types, got %d", len(types))
	}
}

func TestUniqueTaskTypes_Deduplicates(t *testing.T) {
	m := NewManager(&config.Config{})
	m.tasks.Store("t1", &mockTask{id: "t1", typ: "type-a"})
	m.tasks.Store("t2", &mockTask{id: "t2", typ: "type-a"})
	m.tasks.Store("t3", &mockTask{id: "t3", typ: "type-b"})

	types := m.UniqueTaskTypes()
	if len(types) != 2 {
		t.Fatalf("expected 2 unique types, got %d", len(types))
	}
	seen := make(map[string]bool)
	for _, tt := range types {
		seen[tt] = true
	}
	if !seen["type-a"] || !seen["type-b"] {
		t.Fatalf("expected both type-a and type-b, got %v", types)
	}
}

// =============================================================================
// FirstTaskOfType
// =============================================================================

func TestFirstTaskOfType_Found(t *testing.T) {
	m := NewManager(&config.Config{})
	task := &mockTask{id: "t1", typ: "type-a"}
	m.tasks.Store("t1", task)

	found := m.FirstTaskOfType("type-a")
	if found == nil {
		t.Fatal("expected to find task, got nil")
	}
	if found.ID() != "t1" {
		t.Fatalf("expected task ID 't1', got %q", found.ID())
	}
}

func TestFirstTaskOfType_NotFound(t *testing.T) {
	m := NewManager(&config.Config{})
	m.tasks.Store("t1", &mockTask{id: "t1", typ: "type-a"})

	found := m.FirstTaskOfType("type-b")
	if found != nil {
		t.Fatalf("expected nil, got task %q", found.ID())
	}
}

// =============================================================================
// StandardizationService.Run
// =============================================================================

func TestStandardizationService_NoTasks(t *testing.T) {
	m := NewManager(&config.Config{})
	svc := NewStandardizationService(m)
	// Should not panic with no tasks
	svc.Run(t.Context())
}

func TestStandardizationService_NonStandardizerTask(t *testing.T) {
	m := NewManager(&config.Config{})
	// mockTask does NOT implement Standardizer
	m.tasks.Store("t1", &mockTask{id: "t1", typ: "type-a"})
	svc := NewStandardizationService(m)
	// Should not panic, just log a debug message
	svc.Run(t.Context())
}

func TestStandardizationService_ProcessesMissingID(t *testing.T) {
	st, err := storage.NewStorage("memory", nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	// obj1: missing ID (id=0)
	obj1 := &model.DownloadObject{URL: "http://example.com/1", TaskID: "t1"}
	// obj2: already has ID
	obj2 := &model.DownloadObject{URL: "http://example.com/2", TaskID: "t1"}
	obj2.SetID(99)

	if err := st.Update(obj1); err != nil {
		t.Fatalf("Update obj1: %v", err)
	}
	if err := st.Update(obj2); err != nil {
		t.Fatalf("Update obj2: %v", err)
	}

	stdMock := &mockStandardizer{}
	stdMock.id = "t1"
	stdMock.typ = "type-a"
	stdMock.st = st
	// 不需要设置 objs，StandardizationService.Run() 通过 st.Search() 获取对象

	m := NewManager(&config.Config{})
	m.tasks.Store("t1", stdMock)

	svc := NewStandardizationService(m)
	svc.Run(t.Context())

	// obj1 should have been standardized (had missing ID)
	obj1After, _ := st.Get("http://example.com/1")
	if obj1After.GetID() != 42 {
		t.Fatalf("expected obj1 ID to become 42 after standardization, got %d", obj1After.GetID())
	}

	// obj2 should still have ID=99 (already had ID, skipped by MissingID filter)
	obj2After, _ := st.Get("http://example.com/2")
	if obj2After.GetID() != 99 {
		t.Fatalf("expected obj2 ID to remain 99, got %d", obj2After.GetID())
	}
}

func TestStandardizationService_NoMatchingObjects(t *testing.T) {
	st, err := storage.NewStorage("memory", nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	// All objects already have IDs
	obj1 := &model.DownloadObject{URL: "http://example.com/1", TaskID: "t1"}
	obj1.SetID(10)
	obj2 := &model.DownloadObject{URL: "http://example.com/2", TaskID: "t1"}
	obj2.SetID(20)
	if err := st.Update(obj1); err != nil {
		t.Fatalf("Update obj1: %v", err)
	}
	if err := st.Update(obj2); err != nil {
		t.Fatalf("Update obj2: %v", err)
	}

	stdMock := &mockStandardizer{}
	stdMock.id = "t1"
	stdMock.typ = "type-a"
	stdMock.st = st

	m := NewManager(&config.Config{})
	m.tasks.Store("t1", stdMock)

	svc := NewStandardizationService(m)
	svc.Run(t.Context())

	// No objects should have been standardized (all already had IDs)
	if stdMock.callCount != 0 {
		t.Fatalf("expected 0 Standardize calls, got %d", stdMock.callCount)
	}
}

func TestStandardizationService_MultipleTaskTypes(t *testing.T) {
	st1, err := storage.NewStorage("memory", nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	st2, err := storage.NewStorage("memory", nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	// Type-a: one object with missing ID
	obj1 := &model.DownloadObject{URL: "http://a.example.com/1", TaskID: "t1"}
	if err := st1.Update(obj1); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Type-b: one object with missing ID
	obj2 := &model.DownloadObject{URL: "http://b.example.com/1", TaskID: "t2"}
	if err := st2.Update(obj2); err != nil {
		t.Fatalf("Update: %v", err)
	}

	std1 := &mockStandardizer{}
	std1.id = "t1"
	std1.typ = "type-a"
	std1.st = st1

	std2 := &mockStandardizer{}
	std2.id = "t2"
	std2.typ = "type-b"
	std2.st = st2

	m := NewManager(&config.Config{})
	m.tasks.Store("t1", std1)
	m.tasks.Store("t2", std2)

	svc := NewStandardizationService(m)
	svc.Run(t.Context())

	// Both objects should have been standardized
	obj1After, _ := st1.Get("http://a.example.com/1")
	if obj1After.GetID() != 42 {
		t.Fatalf("expected obj1 ID to become 42, got %d", obj1After.GetID())
	}
	obj2After, _ := st2.Get("http://b.example.com/1")
	if obj2After.GetID() != 42 {
		t.Fatalf("expected obj2 ID to become 42, got %d", obj2After.GetID())
	}
}
