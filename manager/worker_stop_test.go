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
// block and no data race occurs.
func TestWorkerStop_ChannelFull(t *testing.T) {
	mgr, _ := newMockManager(t, "worker-stop", 5,
		mockdl.New(mockdl.ModePauseOnProgress, mockdl.WithDelay(10*time.Millisecond)))
	startManager(t, mgr)

	task := waitForTask(t, mgr, "worker-stop")

	// Wait until at least one object enters downloading state.
	var downloading int
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		mgr.scan()
		all := getAllObjectsFromTask(t, task)
		downloading = 0
		for _, obj := range all {
			if obj.GetStatus() == model.StatusDownloading {
				downloading++
			}
		}
		if downloading >= 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if downloading < 1 {
		t.Fatalf("expected at least 1 downloading object, got %d", downloading)
	}
	t.Logf("found %d downloading objects", downloading)

	// Concurrently reduce workers — the workerStop channel should not block.
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

	// Brief pause for goroutines to settle.
	time.Sleep(100 * time.Millisecond)

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
