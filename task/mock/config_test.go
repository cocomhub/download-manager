// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"context"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
	"github.com/cocomhub/download-manager/task"
)

func TestMockRule_Validate(t *testing.T) {
	tests := []struct {
		name    string
		rule    MockRule
		wantErr bool
	}{
		{"valid with count", MockRule{URLTemplate: "http://example.com/file-{n}.bin", Count: 3}, false},
		{"valid with slugs", MockRule{URLTemplate: "http://example.com/{slug}.mp4", Slugs: []string{"a", "b"}}, false},
		{"missing url_template", MockRule{Count: 3}, true},
		{"zero count and no slugs", MockRule{URLTemplate: "http://example.com/f.bin"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestMockRule_GenerateObjects_Basic(t *testing.T) {
	rule := MockRule{
		URLTemplate: "http://example.com/file-{n}.bin",
		Count:       3,
		FileSize:    1024,
		Status:      "pending",
	}

	objects := rule.generateObjects("test-task", 0)
	if len(objects) != 3 {
		t.Fatalf("expected 3 objects, got %d", len(objects))
	}

	expectedURLs := []string{
		"http://example.com/file-0.bin",
		"http://example.com/file-1.bin",
		"http://example.com/file-2.bin",
	}
	for i, obj := range objects {
		if obj.URL != expectedURLs[i] {
			t.Errorf("objects[%d].URL = %q, want %q", i, obj.URL, expectedURLs[i])
		}
		if obj.TaskID != "test-task" {
			t.Errorf("objects[%d].TaskID = %q, want %q", i, obj.TaskID, "test-task")
		}
		if obj.GetStatus() != "pending" {
			t.Errorf("objects[%d].GetStatus() = %q, want %q", i, obj.GetStatus(), "pending")
		}
	}
}

func TestMockRule_GenerateObjects_WithSlugs(t *testing.T) {
	rule := MockRule{
		URLTemplate: "http://example.com/{slug}.mp4",
		Slugs:       []string{"ep1", "ep2", "ep3"},
		Metadata:    map[string]string{"content_group": "test-show"},
	}

	objects := rule.generateObjects("test-task", 0)
	if len(objects) != 3 {
		t.Fatalf("expected 3 objects, got %d", len(objects))
	}

	expectedURLs := []string{
		"http://example.com/ep1.mp4",
		"http://example.com/ep2.mp4",
		"http://example.com/ep3.mp4",
	}
	for i, obj := range objects {
		if obj.URL != expectedURLs[i] {
			t.Errorf("objects[%d].URL = %q, want %q", i, obj.URL, expectedURLs[i])
		}
		if obj.Metadata[model.MetadataKeyContentGroup] != "test-show" {
			t.Errorf("objects[%d].Metadata content_group = %q, want %q", i, obj.Metadata[model.MetadataKeyContentGroup], "test-show")
		}
	}
}

func TestMockRule_GenerateObjects_WithOffset(t *testing.T) {
	rule := MockRule{
		URLTemplate:     "http://example.com/file-{n}.bin",
		Count:           2,
		InitialProgress: 45,
		Status:          "downloading",
	}

	objects := rule.generateObjects("test-task", 5)
	if len(objects) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objects))
	}
	if objects[0].URL != "http://example.com/file-5.bin" {
		t.Errorf("objects[0].URL = %q, want %q", objects[0].URL, "http://example.com/file-5.bin")
	}
	if objects[1].URL != "http://example.com/file-6.bin" {
		t.Errorf("objects[1].URL = %q, want %q", objects[1].URL, "http://example.com/file-6.bin")
	}
	if objects[0].GetStatus() != "downloading" {
		t.Errorf("objects[0].GetStatus() = %q, want %q", objects[0].GetStatus(), "downloading")
	}
}

func TestParseMockRules(t *testing.T) {
	extra := map[string]any{
		"mock_rules": []any{
			map[string]any{
				"url_template": "http://example.com/f-{n}.bin",
				"count":        2,
				"file_size":    2048,
			},
		},
	}

	rules, err := parseMockRules(extra)
	if err != nil {
		t.Fatalf("parseMockRules() error = %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].URLTemplate != "http://example.com/f-{n}.bin" {
		t.Errorf("URLTemplate = %q, want %q", rules[0].URLTemplate, "http://example.com/f-{n}.bin")
	}
}

func TestParseMockRules_Missing(t *testing.T) {
	_, err := parseMockRules(map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing mock_rules")
	}
}

func TestParseMockBehavior(t *testing.T) {
	extra := map[string]any{
		"mock_behavior": map[string]any{
			"mode":      "simulate_progress",
			"fail_rate": 0.5,
		},
	}

	b, err := parseMockBehavior(extra)
	if err != nil {
		t.Fatalf("parseMockBehavior() error = %v", err)
	}
	if b.Mode != "simulate_progress" {
		t.Errorf("Mode = %q, want %q", b.Mode, "simulate_progress")
	}
	if b.FailRate != 0.5 {
		t.Errorf("FailRate = %f, want 0.5", b.FailRate)
	}
}

func TestParseMockBehavior_Default(t *testing.T) {
	b, err := parseMockBehavior(map[string]any{})
	if err != nil {
		t.Fatalf("parseMockBehavior() error = %v", err)
	}
	if b.Mode != "" {
		t.Errorf("Mode = %q, want empty", b.Mode)
	}
}

func TestNewMockTask_Type(t *testing.T) {
	mockT := newMockTaskFromConfig(t, 2)
	if mockT.Type() != "mock" {
		t.Errorf("Type() = %q, want %q", mockT.Type(), "mock")
	}
}

func TestNewMockTask_GetDownloadObjects(t *testing.T) {
	mockT := newMockTaskFromConfig(t, 3)

	objects, err := mockT.GetDownloadObjects()
	if err != nil {
		t.Fatalf("GetDownloadObjects() error = %v", err)
	}

	if len(objects) != 3 {
		t.Errorf("expected 3 objects, got %d", len(objects))
	}

	// Second call should be idempotent.
	objects2, err := mockT.GetDownloadObjects()
	if err != nil {
		t.Fatalf("GetDownloadObjects() (second call) error = %v", err)
	}
	if len(objects2) != 3 {
		t.Errorf("expected 3 objects on second call, got %d", len(objects2))
	}
}

func TestNewMockTask_GetDownloadObjects_FiltersCompleted(t *testing.T) {
	mockT := newMockTaskFromConfig(t, 2)
	st := mockT.Storage()

	// Seed first.
	objs, err := mockT.GetDownloadObjects()
	if err != nil {
		t.Fatalf("GetDownloadObjects() error = %v", err)
	}

	// Mark the first object as completed via UpdateStatus.
	if len(objs) > 0 {
		obj := objs[0]
		obj.SetStatus(model.StatusCompleted)
		obj.SetProgress(100)
		_ = st.Update(obj)
	}

	// Now GetDownloadObjects should exclude completed.
	objects, err := mockT.GetDownloadObjects()
	if err != nil {
		t.Fatalf("GetDownloadObjects() error = %v", err)
	}

	for _, obj := range objects {
		if obj.GetStatus() == model.StatusCompleted {
			t.Errorf("GetDownloadObjects() returned completed object: %s", obj.URL)
		}
	}

	// Should have N-1 objects.
	if len(objects) != 1 {
		t.Errorf("expected 1 pending object (one completed), got %d", len(objects))
	}
}

func TestNewMockTask_GetDownloadObjects_WithSlugs(t *testing.T) {
	extra := map[string]any{
		"mock_rules": []any{
			map[string]any{
				"url_template": "http://example.com/{slug}.mp4",
				"slugs":        []any{"a", "b", "c"},
			},
		},
	}
	mockT := newMockTaskFromRawExtra(t, extra)

	objects, err := mockT.GetDownloadObjects()
	if err != nil {
		t.Fatalf("GetDownloadObjects() error = %v", err)
	}
	if len(objects) != 3 {
		t.Errorf("expected 3 objects from slugs, got %d", len(objects))
	}
}

func TestNewMockTask_Scrape_Noop(t *testing.T) {
	mockT := newMockTaskFromConfig(t, 2)

	// Ensure seeded.
	_, _ = mockT.GetDownloadObjects()
	objs1 := mockT.GetAllObjects(true)

	var sc core.ScrapeCap = mockT

	err := sc.Scrape(t.Context())
	if err != nil {
		t.Fatalf("Scrape() error = %v", err)
	}

	// Second scrape.
	err = sc.Scrape(t.Context())
	if err != nil {
		t.Fatalf("Scrape() error = %v", err)
	}

	objs2 := mockT.GetAllObjects(true)
	if len(objs1) != len(objs2) {
		t.Errorf("Scrape should be no-op when refresh_interval=0, objects changed: %d -> %d", len(objs1), len(objs2))
	}
}

func TestNewMockTask_Scrape_Cancelled(t *testing.T) {
	mockT := newMockTaskFromConfig(t, 2)
	var sc core.ScrapeCap = mockT

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := sc.Scrape(ctx)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestNewMockTask_ResolveObject(t *testing.T) {
	mockT := newMockTaskFromConfig(t, 1)

	obj := &model.DownloadObject{URL: "http://example.com/test.bin"}
	err := mockT.ResolveObject(t.Context(), obj)
	if err != nil {
		t.Fatalf("ResolveObject() error = %v", err)
	}
}

// TestNewMockTask_GetAllObjects tests the embedded GetAllObjects method.
func TestNewMockTask_GetAllObjects(t *testing.T) {
	mockT := newMockTaskFromConfig(t, 3)

	_, _ = mockT.GetDownloadObjects()
	all := mockT.GetAllObjects(true)

	if len(all) != 3 {
		t.Errorf("expected 3 total objects, got %d", len(all))
	}
}

// --- helpers ---

func newMockTaskFromConfig(t *testing.T, count int) *Task {
	t.Helper()
	return newMockTaskFromRawExtra(t, map[string]any{
		"mock_rules": []any{
			map[string]any{
				"url_template": "http://example.com/file-{n}.bin",
				"count":        count,
			},
		},
	})
}

func newMockTaskFromRawExtra(t *testing.T, extra map[string]any) *Task {
	t.Helper()
	s, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatalf("NewMemoryStorage: %v", err)
	}

	cfg := config.Task{
		ID:      "mock-test",
		Type:    "mock",
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
		Extra:   extra,
	}

	// Use WithStore to inject the pre-created storage.
	mockT, err := task.NewTask(&cfg, task.WithStore(s))
	if err != nil {
		t.Fatalf("NewTask(mock) error = %v", err)
	}

	return mockT.(*Task)
}
