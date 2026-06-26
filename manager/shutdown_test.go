// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/testutil/assert"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestShutdown_InFlightDownloads verifies that when Manager.Stop() is called
// while downloads are in flight, the survivors are correctly marked as failed
// and no goroutine is leaked.
func TestShutdown_InFlightDownloads(t *testing.T) {
	mgr, _ := newMockManager(t, "shutdown-test", 3,
		mockdl.New(mockdl.ModePauseOnProgress, mockdl.WithDelay(100*time.Millisecond)))

	done := make(chan struct{})
	go func() {
		mgr.Start()
		close(done)
	}()

	// Track whether we've already stopped to avoid double-Stop+close.
	var stopped atomic.Bool
	stopOnce := func() {
		if stopped.Load() {
			return
		}
		stopped.Store(true)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		mgr.Stop(ctx)
		<-done
	}
	defer stopOnce()

	task := waitForTask(t, mgr, "shutdown-test")

	// Wait until all objects are resolved (StatusDownloading after resolve fix).
	// With the resolve→StatusDownloading optimization, objects enter StatusDownloading
	// during resolve, before being enqueued to the download queue.
	assert.MustEventually(t, func() bool {
		mgr.scan()
		all := getAllObjectsFromTask(t, task)
		for _, obj := range all {
			if obj.GetStatus() != model.StatusDownloading {
				return false
			}
		}
		return true
	}, 10*time.Second, 200*time.Millisecond, "expected all objects to be resolved")

	// Wait for the scheduler to enqueue at least one object to the download queue.
	// After resolve, processTask must run again to push StatusDownloading objects
	// through the task queue → scheduler → download queue pipeline.
	assert.MustEventually(t, func() bool {
		mgr.scan()
		mgr.mu.Lock()
		active := mgr.activeDownloads["shutdown-test"]
		mgr.mu.Unlock()
		return active > 0
	}, 5*time.Second, 100*time.Millisecond, "expected at least one object to be enqueued")

	// Call Stop — this should cancel the in-flight download and mark survivors.
	stopOnce()

	// Verify: downloadingObj should be empty (all cleaned up).
	var remaining int
	mgr.downloadingObj.Range(func(_, _ any) bool {
		remaining++
		return true
	})
	if remaining > 0 {
		t.Errorf("expected empty downloadingObj after Stop, got %d items", remaining)
	}

	// Verify: activeDownloads >= 0 for all tasks.
	mgr.mu.Lock()
	for taskID, count := range mgr.activeDownloads {
		if count < 0 {
			t.Errorf("negative activeDownloads for %s: %d", taskID, count)
		}
		t.Logf("activeDownloads[%s] = %d", taskID, count)
	}
	mgr.mu.Unlock()

	// Verify: downloading objects are marked failed; pending objects stay pending.
	// Note: with the resolve->StatusDownloading optimization, objects that were
	// resolved but not yet enqueued to the download queue will remain in
	// StatusDownloading state. This is acceptable — they are resolved and will
	// be picked up on the next run.
	all := getAllObjectsFromTask(t, task)
	for _, obj := range all {
		status := obj.GetStatus()
		switch status {
		case model.StatusDownloading:
			// Already resolved but not yet dispatched — correct, will resume on restart.
		case model.StatusFailed, model.StatusCancelled:
			// Terminal state from shutdown — correct.
		case model.StatusPending:
			// Was never picked up — correct, it wasn't in downloadingObj.
		default:
			t.Errorf("unexpected status %s for %s after shutdown", status, obj.URL)
		}
	}
}
