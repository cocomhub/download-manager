// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task_test

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/task"
)

// standardizerRecorder records whether Standardize was called and sets a known ID.
type standardizerRecorder struct {
	called bool
}

func (r *standardizerRecorder) Standardize(obj *model.DownloadObject) (bool, error) {
	r.called = true
	obj.SetID(42)
	return true, nil
}

var _ core.Standardizer = (*standardizerRecorder)(nil)

func TestBaseTask_RememberRuntimeObject_CallsStandardizer(t *testing.T) {
	t.Parallel()

	rec := &standardizerRecorder{}
	bt, err := task.NewBaseTask(&config.Task{
		ID:      "t1",
		Type:    "base",
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
	}, task.Options{})
	if err != nil {
		t.Fatalf("NewBaseTask error: %v", err)
	}

	bt.SetSelf(rec)

	obj := &model.DownloadObject{
		TaskID: bt.ID(),
		URL:    "http://example.com/video",
		Status: model.StatusPending,
		Extra:  map[string]any{},
	}

	bt.RememberRuntimeObject(obj, true)

	if !rec.called {
		t.Fatal("expected Standardizer.Standardize to be called")
	}
	if obj.GetID() != 42 {
		t.Fatalf("expected object ID to be 42 after standardize, got %d", obj.GetID())
	}
}

func TestBaseTask_RememberRuntimeObject_SkipsStandardizerIfNotImplemented(t *testing.T) {
	t.Parallel()

	bt, err := task.NewBaseTask(&config.Task{
		ID:      "t1",
		Type:    "base",
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
	}, task.Options{})
	if err != nil {
		t.Fatalf("NewBaseTask error: %v", err)
	}

	// No SetSelf call — self is nil, so Standardizer should not be invoked
	obj := &model.DownloadObject{
		TaskID: bt.ID(),
		URL:    "http://example.com/video",
		Status: model.StatusPending,
		Extra:  map[string]any{},
	}

	// Should not panic
	bt.RememberRuntimeObject(obj, true)
	if obj.GetID() != 0 {
		t.Fatalf("expected object ID to remain 0, got %d", obj.GetID())
	}
}

func TestBaseTask_RememberRuntimeObject_NilSafe(t *testing.T) {
	t.Parallel()

	bt, err := task.NewBaseTask(&config.Task{
		ID:      "t1",
		Type:    "base",
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
	}, task.Options{})
	if err != nil {
		t.Fatalf("NewBaseTask error: %v", err)
	}

	// Should not panic
	bt.RememberRuntimeObject(nil, true)
	bt.RememberRuntimeObject(nil, false)
}

// TestBaseTask_SetSelf_And_Standardizer verifies that SetSelf stores the reference
// and that the Standardizer interface works when called directly via the stored reference.
func TestBaseTask_SetSelf_And_Standardizer(t *testing.T) {
	t.Parallel()

	rec := &standardizerRecorder{}
	bt, err := task.NewBaseTask(&config.Task{
		ID:      "t1",
		Type:    "base",
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
	}, task.Options{})
	if err != nil {
		t.Fatalf("NewBaseTask error: %v", err)
	}

	bt.SetSelf(rec)

	// Verify that Standardizer interface assertion works via any
	var std core.Standardizer = rec
	obj := &model.DownloadObject{
		URL: "http://example.com/test",
	}

	modified, err := std.Standardize(obj)
	if err != nil {
		t.Fatalf("Standardize error: %v", err)
	}
	if !modified {
		t.Fatal("expected modified=true")
	}
	if obj.GetID() != 42 {
		t.Fatalf("expected ID 42, got %d", obj.GetID())
	}
}

func TestBaseTask_ResetZombieState_OnlyResetsDownloading(t *testing.T) {
	bt, err := task.NewBaseTask(&config.Task{
		ID:      "t1",
		Type:    "base",
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
	}, task.Options{})
	if err != nil {
		t.Fatalf("NewBaseTask error: %v", err)
	}

	cases := []struct {
		status string
		want   string
	}{
		{status: model.StatusDownloading, want: model.StatusPending},
		{status: model.StatusPending, want: model.StatusPending},
		{status: model.StatusFailed, want: model.StatusFailed},
		{status: model.StatusCompleted, want: model.StatusCompleted},
		{status: model.StatusCancelled, want: model.StatusCancelled},
	}

	for _, tc := range cases {
		url := "http://example.com/" + tc.status
		obj := &model.DownloadObject{
			TaskID: bt.ID(),
			URL:    url,
			Status: tc.status,
			Extra:  map[string]any{},
		}

		if err := bt.Storage().Update(obj); err != nil {
			t.Fatalf("seed storage err: %v", err)
		}

		bt.ResetZombieState(obj)

		if obj.GetStatus() != tc.want {
			t.Fatalf("status=%s: expected obj status %s, got %s", tc.status, tc.want, obj.GetStatus())
		}

		stored, err := bt.Storage().Get(url)
		if err != nil {
			t.Fatalf("get storage err: %v", err)
		}
		if stored == nil {
			t.Fatalf("expected stored obj")
		}
		if stored.GetStatus() != tc.want {
			t.Fatalf("status=%s: expected stored status %s, got %s", tc.status, tc.want, stored.Status)
		}
	}
}
