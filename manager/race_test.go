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

// TestRace_ActiveDownloadsNegative verifies that concurrent CancelTask + forceDownload
// does not cause activeDownloads to go negative.
func TestRace_ActiveDownloadsNegative(t *testing.T) {
	mgr, _ := newMockManager(t, "race-ad", 10,
		mockdl.New(mockdl.ModePauseOnProgress, mockdl.WithDelay(50*time.Millisecond)))
	startManager(t, mgr)

	task := waitForTask(t, mgr, "race-ad")

	// Wait until at least 5 objects enter downloading state.
	deadline := time.Now().Add(5 * time.Second)
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
		if downloading >= 5 {
			t.Logf("found %d downloading objects", downloading)
			break
		}
	}

	// Concurrently cancel and force-download.
	// Make downloads complete quickly so CancelTask and retries race.
	// Switch downloader to always-success mode for the race.
	mgr.downloader = mockdl.New(mockdl.ModeAlwaysSuccess)

	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			_ = mgr.CancelTask("race-ad")
		})
		wg.Go(func() {
			all := getAllObjectsFromTask(t, task)
			for _, obj := range all {
				_ = mgr.RetryObject("race-ad", obj.URL)
			}
		})
	}
	wg.Wait()

	// Wait for activeDownloads to settle.
	time.Sleep(500 * time.Millisecond)

	// Verify activeDownloads >= 0 for all tasks.
	mgr.mu.Lock()
	for taskID, count := range mgr.activeDownloads {
		if count < 0 {
			t.Errorf("activeDownloads[%s] = %d (negative!)", taskID, count)
		}
		t.Logf("activeDownloads[%s] = %d", taskID, count)
	}
	mgr.mu.Unlock()
}
