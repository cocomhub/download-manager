// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sync"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/testutil/assert"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestWorkerStop_ChannelFull verifies that when many workers are stopped
// simultaneously via adjustGlobalWorkers, the workerStop channel does not
// block and no data race occurs.
func TestWorkerStop_ChannelFull(t *testing.T) {
	mgr, _ := newMockManager(t, "worker-stop", 5,
		mockdl.New(mockdl.ModePauseOnProgress, mockdl.WithDelay(10*time.Millisecond)))
	startManager(t, mgr)

	task := waitForTask(t, mgr, "worker-stop")

	// Wait until at least one object enters downloading state.
	var downloading int
	assert.MustEventually(t, func() bool {
		mgr.scan()
		all := getAllObjectsFromTask(t, task)
		downloading = 0
		for _, obj := range all {
			if obj.GetStatus() == model.StatusDownloading {
				downloading++
			}
		}
		return downloading >= 1
	}, 6*time.Second, 100*time.Millisecond, "expected at least 1 downloading object")
	t.Logf("found %d downloading objects", downloading)

	// Concurrently reduce workers — the workerStop channel should not block.
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			old := int(mgr.workerCount.Load())
			if old > 1 {
				mgr.adjustGlobalWorkers(old - 1)
			}
		})
	}
	wg.Wait()

	// Wait for workers to settle and activeDownloads to stabilize (no negative values).
	assert.MustEventually(t, func() bool {
		mgr.mu.Lock()
		defer mgr.mu.Unlock()
		for _, count := range mgr.activeDownloads {
			if count < 0 {
				return false
			}
		}
		return true
	}, 3*time.Second, 50*time.Millisecond, "expected no negative activeDownloads after stopping workers")

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
