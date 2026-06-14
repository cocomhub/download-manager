// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/model"
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

	// Wait until at least one object enters downloading state.
	deadline := time.Now().Add(5 * time.Second)
	var found int
	for time.Now().Before(deadline) {
		mgr.scan()
		time.Sleep(200 * time.Millisecond)

		all := getAllObjectsFromTask(t, task)
		found = 0
		for _, obj := range all {
			if obj.GetStatus() == model.StatusDownloading {
				found++
			}
		}
		if found >= 1 {
			t.Logf("found %d downloading objects, proceeding with Stop", found)
			break
		}
	}
	if found == 0 {
		t.Fatal("no object entered downloading state within timeout")
	}

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
	all := getAllObjectsFromTask(t, task)
	for _, obj := range all {
		status := obj.GetStatus()
		switch status {
		case model.StatusDownloading:
			// Should have been caught by Stop() survivor marking — but download defer
			// already transitioned it to failed via context cancel. Either is OK.
			t.Errorf("unexpected downloading state after Stop for %s", obj.URL)
		case model.StatusFailed, model.StatusCancelled:
			// Terminal state from shutdown — correct.
		case model.StatusPending:
			// Was never picked up — correct, it wasn't in downloadingObj.
		default:
			t.Errorf("unexpected status %s for %s after shutdown", status, obj.URL)
		}
	}
}
