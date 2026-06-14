// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sync"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/model"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestWorkerStop_ChannelFull verifies that when many workers are stopped
// simultaneously via adjustGlobalWorkers, the workerStop channel does not
// block (it must have sufficient capacity or non-blocking semantics).
func TestWorkerStop_ChannelFull(t *testing.T) {
	mgr, _ := newMockManager(t, "worker-stop", 5,
		mockdl.New(mockdl.ModePauseOnProgress, mockdl.WithDelay(10*time.Millisecond)))
	startManager(t, mgr)

	task := waitForTask(t, mgr, "worker-stop")

	// Let some objects enter downloading state.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mgr.scan()
		time.Sleep(100 * time.Millisecond)

		all := getAllObjectsFromTask(t, task)
		var downloading int
		for _, obj := range all {
			if obj.GetStatus() == model.StatusDownloading {
				downloading++
			}
		}
		if downloading >= 1 {
			t.Logf("found %d downloading objects", downloading)
			break
		}
	}

	// Perform many concurrent worker count adjustments in sequence
	// to ensure workerStop channel doesn't block adjustGlobalWorkers.
	// AdjustGlobalWorkers now holds mu internally, so read count outside.
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(step int) {
			defer wg.Done()
			mgr.mu.Lock()
			old := mgr.workerCount
			mgr.mu.Unlock()
			if old > 1 {
				mgr.adjustGlobalWorkers(old - 1)
			}
		}(i)
	}
	wg.Wait()

	// Small sleep to let workers drain.
	time.Sleep(100 * time.Millisecond)

	all := getAllObjectsFromTask(t, task)
	t.Logf("after worker stop test: %d objects in task", len(all))

	// Verify activeDownloads never went negative.
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	for taskID, count := range mgr.activeDownloads {
		if count < 0 {
			t.Errorf("activeDownloads[%s] = %d (negative!)", taskID, count)
		}
		t.Logf("activeDownloads[%s] = %d", taskID, count)
	}
}
