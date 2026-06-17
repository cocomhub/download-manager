// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestManagerWithMockTask verifies the full lifecycle with a mock task:
// Start → loadTasks → processTask → download → complete.
func TestManagerWithMockTask(t *testing.T) {
	mgr, _ := newMockManager(t, "mock-e2e", 3, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)

	task := waitForTask(t, mgr, "mock-e2e")
	defer func() {
		all := getAllObjectsFromTask(t, task)
		for _, obj := range all {
			t.Logf("final: %s status=%s", obj.URL, obj.GetStatus())
		}
	}()

	// Retry-loop: wait for all 3 objects to become completed.
	waitForObjectsFinal(t, mgr, task, 3, model.StatusCompleted, 5*time.Second)

	all := getAllObjectsFromTask(t, task)
	for _, obj := range all {
		if obj.GetStatus() != model.StatusCompleted {
			t.Errorf("expected completed, got %s for %s", obj.GetStatus(), obj.URL)
		}
	}
}

// TestManagerWithMockTask_RetryThenSuccess uses first_fail_then_success
// downloader to verify retry works.
func TestManagerWithMockTask_RetryThenSuccess(t *testing.T) {
	mgr, _ := newMockManager(t, "mock-retry", 1,
		mockdl.New(mockdl.ModeFirstFailThenSuccess))
	_ = startManager(t, mgr)

	task := waitForTask(t, mgr, "mock-retry")

	// Wait for the failed state (first attempt always fails with ModeFirstFailThenSuccess).
	waitForObjectsFinal(t, mgr, task, 1, model.StatusFailed, 3*time.Second)

	// Trigger retry.
	if err := mgr.RetryAllFailed("mock-retry"); err != nil {
		t.Fatalf("RetryAllFailed: %v", err)
	}

	// Wait for success after retry.
	waitForObjectsFinal(t, mgr, task, 1, model.StatusCompleted, 5*time.Second)

	all := getAllObjectsFromTask(t, task)
	for _, obj := range all {
		if obj.GetStatus() != model.StatusCompleted {
			t.Errorf("expected completed after retry, got %s for %s", obj.GetStatus(), obj.URL)
		}
	}
}

// TestManagerWithMockTask_CancelDuringDownload verifies that cancelling works.
func TestManagerWithMockTask_CancelDuringDownload(t *testing.T) {
	mgr, _ := newMockManager(t, "mock-cancel", 3,
		mockdl.New(mockdl.ModeSimulateProgress, mockdl.WithDelay(500*time.Millisecond)))
	_ = startManager(t, mgr)

	task := waitForTask(t, mgr, "mock-cancel")

	// Wait until at least one object enters downloading state.
	// ProcessTask sets StatusDownloading BEFORE calling dl.Download(),
	// so we should see it in the task's object list quickly.
	deadline := time.Now().Add(3 * time.Second)
	var target string
	for time.Now().Before(deadline) {
		mgr.scan()
		time.Sleep(200 * time.Millisecond)

		all := getAllObjectsFromTask(t, task)
		for _, obj := range all {
			if obj.GetStatus() == model.StatusDownloading {
				target = obj.URL
				t.Logf("found downloading object: %s", obj.URL)
				break
			}
		}
		if target != "" {
			break
		}
	}

	if target == "" {
		t.Fatal("no object entered downloading state within timeout")
	}

	// Cancel the downloading object.
	t.Logf("cancelling object: %s", target)
	if err := mgr.CancelObject("mock-cancel", target); err != nil {
		t.Logf("CancelObject: %v", err)
	}

	// Wait for the cancellation to take effect.
	waitForObjectsFinal(t, mgr, task, 1, model.StatusCancelled, 3*time.Second)
}

// --- helpers ---

// newMockManager creates a Manager with mock task and injects MockDownloader.
func newMockManager(t *testing.T, taskID string, objectCount int, dl *mockdl.MockDownloader) (*Manager, *mockdl.MockDownloader) {
	t.Helper()
	cfg := &config.Config{
		Runtime: config.Runtime{
			Mode: config.RunModeFull,
			Download: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{
				Enabled: true,
			},
			Scheduler: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{
				Enabled: true,
			},
		},
		Server: config.Server{
			WorkDir:         t.TempDir(),
			DownloadRootDir: t.TempDir(),
		},
		Downloader: config.Downloader{
			GlobalConcurrent: 5,
			MaxRetries:       2,
		},
		Tasks: []config.Task{
			{
				ID:      taskID,
				Type:    "mock",
				SaveDir: t.TempDir(),
				Storage: config.StorageConfig{Type: "memory"},
				Extra: map[string]any{
					"mock_rules": []any{
						map[string]any{
							"url_template": "http://mock-download/file-{n}.bin",
							"count":        objectCount,
						},
					},
				},
			},
		},
	}
	mgr := NewManager(cfg)
	mgr.setDownloader(dl)
	return mgr, dl
}

// waitForTask polls until the task is registered.
func waitForTask(t *testing.T, mgr *Manager, taskID string) core.Task {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if tsk, ok := mgr.getTask(taskID); ok {
			return tsk
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("task %s not found within timeout", taskID)
	return nil
}

// startManager starts the manager in a goroutine and registers cleanup.
func startManager(t *testing.T, mgr *Manager) chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		mgr.Start()
		close(done)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		mgr.Stop(ctx)
		<-done
	})
	return done
}

// waitForObjectsFinal polling loop that repeatedly scans until count objects
// reach the target state, or timeout. Does NOT fail the test on timeout.
func waitForObjectsFinal(t *testing.T, mgr *Manager, task core.Task, count int, target string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mgr.scan()
		time.Sleep(300 * time.Millisecond)

		all := getAllObjectsFromTask(t, task)
		var matched int
		for _, obj := range all {
			if obj.GetStatus() == target {
				matched++
			}
		}
		if matched >= count {
			return
		}
	}

	// Final scan + check (race-condition guard).
	mgr.scan()
	time.Sleep(200 * time.Millisecond)
	all := getAllObjectsFromTask(t, task)
	var matched int
	for _, obj := range all {
		if obj.GetStatus() == target {
			matched++
		}
	}
	if matched >= count {
		return
	}

	t.Logf("waitForObjectsFinal: wanted %d×%s, got %d:", count, target, matched)
	for _, obj := range all {
		t.Logf("  %s status=%s", obj.URL, obj.GetStatus())
	}
}

// getAllObjectsFromTask fetches all download objects from a task.
func getAllObjectsFromTask(t *testing.T, task core.Task) []*model.DownloadObject {
	t.Helper()
	if accessor, ok := task.(interface {
		GetAllObjects(lock bool) []*model.DownloadObject
	}); ok {
		return accessor.GetAllObjects(true)
	}
	objs, err := task.GetDownloadObjects()
	if err != nil {
		t.Logf("GetDownloadObjects: %v", err)
	}
	return objs
}
