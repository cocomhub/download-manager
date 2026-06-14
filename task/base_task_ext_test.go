// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task_test

import (
	"context"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/task"
)

// TestCheckAndRestoreStatus_SharedRegistryFirst verifies shared registry takes
// priority over local storage when restoring status.
func TestCheckAndRestoreStatus_SharedRegistryFirst(t *testing.T) {
	bt := newBaseTaskForTest(t)
	reg := &memSharedRegistry{m: make(map[string]*model.DownloadObject)}
	bt.SetSharedRegistry(reg)

	obj := &model.DownloadObject{
		URL:    "http://test/obj",
		Status: model.StatusPending,
	}

	// Object doesn't exist anywhere yet — should stay pending.
	bt.CheckAndRestoreStatus(obj)
	if obj.GetStatus() != model.StatusPending {
		t.Errorf("expected pending, got %s", obj.GetStatus())
	}

	// Store completed in storage
	bt.Storage().Update(&model.DownloadObject{
		URL:    "http://test/obj",
		Status: model.StatusCompleted,
	})

	// Store cancelled in shared registry
	reg.Update(&model.DownloadObject{
		URL:    "http://test/obj",
		Status: model.StatusCancelled,
	})

	// Shared registry should win
	bt.CheckAndRestoreStatus(obj)
	if obj.GetStatus() != model.StatusCancelled {
		t.Errorf("expected cancelled from shared registry, got %s", obj.GetStatus())
	}
}

// TestCheckAndRestoreStatus_FallbackToStorage verifies that when shared registry
// has no entry, the status falls back to storage.
func TestCheckAndRestoreStatus_FallbackToStorage(t *testing.T) {
	bt := newBaseTaskForTest(t)
	reg := &memSharedRegistry{m: make(map[string]*model.DownloadObject)}
	bt.SetSharedRegistry(reg)

	obj := &model.DownloadObject{
		URL:    "http://test/obj2",
		Status: model.StatusPending,
	}

	// Store completed in storage only
	bt.Storage().Update(&model.DownloadObject{
		URL:    "http://test/obj2",
		Status: model.StatusCompleted,
	})

	bt.CheckAndRestoreStatus(obj)
	if obj.GetStatus() != model.StatusCompleted {
		t.Errorf("expected completed from storage, got %s", obj.GetStatus())
	}
}

// TestCheckRestoreCompleted_IgnoresNonCompleted verifies that CheckRestoreCompleted
// only restores completed objects, not pending or failed ones.
func TestCheckRestoreCompleted_IgnoresNonCompleted(t *testing.T) {
	bt := newBaseTaskForTest(t)
	reg := &memSharedRegistry{m: make(map[string]*model.DownloadObject)}
	bt.SetSharedRegistry(reg)

	obj := &model.DownloadObject{
		URL:    "http://test/obj3",
		Status: model.StatusPending,
	}

	// Store failed in storage
	bt.Storage().Update(&model.DownloadObject{
		URL:    "http://test/obj3",
		Status: model.StatusFailed,
	})

	// Should NOT restore — obj stays pending
	bt.CheckRestoreCompleted(obj)
	if obj.GetStatus() != model.StatusPending {
		t.Errorf("expected pending (non-completed not restored), got %s", obj.GetStatus())
	}
}

// TestResetZombieState_ResetsDownloading verifies ResetZombieState converts
// StatusDownloading -> StatusPending and updates storage.
func TestResetZombieState_ResetsDownloading(t *testing.T) {
	bt := newBaseTaskForTest(t)
	reg := &memSharedRegistry{m: make(map[string]*model.DownloadObject)}
	bt.SetSharedRegistry(reg)

	bt.Storage().Update(&model.DownloadObject{
		URL:    "http://test/zombie",
		Status: model.StatusDownloading,
	})

	obj := &model.DownloadObject{
		URL:    "http://test/zombie",
		Status: model.StatusDownloading,
	}

	bt.ResetZombieState(obj)
	if obj.GetStatus() != model.StatusPending {
		t.Errorf("expected pending after reset, got %s", obj.GetStatus())
	}

	// Verify storage was also updated
	stored, _ := bt.Storage().Get("http://test/zombie")
	if stored.GetStatus() != model.StatusPending {
		t.Errorf("expected pending in storage, got %s", stored.GetStatus())
	}
}

// TestMarkAsFailed_Permanent captures the failed_permanent transition.
func TestMarkAsFailed_Permanent(t *testing.T) {
	bt := newBaseTaskForTest(t)
	reg := &memSharedRegistry{m: make(map[string]*model.DownloadObject)}
	bt.SetSharedRegistry(reg)

	obj := &model.DownloadObject{
		URL:    "http://test/fail-perm",
		Status: model.StatusDownloading,
	}
	err := bt.UpdateStatus(obj, model.StatusDownloading, nil)
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	bt.MarkAsFailed(obj, context.DeadlineExceeded)
	if obj.GetStatus() != model.StatusFailedPermanent {
		t.Errorf("expected failed_permanent, got %s", obj.GetStatus())
	}

	if !bt.IsMarkedFailed(obj.URL) {
		t.Errorf("IsMarkedFailed should return true for failed_permanent")
	}
}

// TestSetConcurrency_Boundaries verifies concurrency clamping.
func TestSetConcurrency_Boundaries(t *testing.T) {
	bt := newBaseTaskForTest(t)

	tests := []struct {
		input   int
		wantErr bool
	}{
		{0, false},
		{50, false},
		{100, false},
		{-1, true},
		{101, true},
	}

	for _, tc := range tests {
		err := bt.SetConcurrency(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("SetConcurrency(%d): wantErr=%v, got %v", tc.input, tc.wantErr, err)
		}
	}
}

// TestSetRefreshInterval_Boundaries verifies refresh interval clamping.
func TestSetRefreshInterval_Boundaries(t *testing.T) {
	bt := newBaseTaskForTest(t)

	tests := []struct {
		input   int
		wantErr bool
	}{
		{10, false},
		{3600, false},
		{86400, false},
		{9, true},
		{86401, true},
	}

	for _, tc := range tests {
		err := bt.SetRefreshInterval(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("SetRefreshInterval(%d): wantErr=%v, got %v", tc.input, tc.wantErr, err)
		}
	}
}

// newBaseTaskForTest creates a BaseTask with memory storage for unit tests.
func newBaseTaskForTest(t *testing.T) *task.BaseTask {
	t.Helper()

	cfg := &config.Task{
		ID:      "test-bt",
		Type:    "test",
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
		Extra: map[string]any{
			"max_concurrent":   5,
			"refresh_interval": 60,
		},
	}

	bt, err := task.NewBaseTask(cfg, task.Options{})
	if err != nil {
		t.Fatalf("NewBaseTask: %v", err)
	}

	return bt
}

// memSharedRegistry is an in-memory shared registry for test use.
type memSharedRegistry struct {
	m map[string]*model.DownloadObject
}

func (r *memSharedRegistry) Get(url string) (*model.DownloadObject, error) {
	if v, ok := r.m[url]; ok {
		return v, nil
	}
	return nil, nil
}

func (r *memSharedRegistry) Update(obj *model.DownloadObject) error {
	r.m[obj.URL] = obj
	return nil
}

func (r *memSharedRegistry) Delete(url string) error {
	delete(r.m, url)
	return nil
}

// Ensure compile-time check for interface.
var _ core.SharedRegistry = (*memSharedRegistry)(nil)
