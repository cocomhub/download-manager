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

// TestRace_ActiveDownloadsNegative verifies that concurrent CancelTask + forceDownload
// does not cause activeDownloads to go negative.
func TestRace_ActiveDownloadsNegative(t *testing.T) {
	mgr, _ := newMockManager(t, "race-ad", 10,
		mockdl.New(mockdl.ModePauseOnProgress, mockdl.WithDelay(50*time.Millisecond)))
	startManager(t, mgr)

	task := waitForTask(t, mgr, "race-ad")

	// Wait until at least 2 objects enter downloading state (limited by
	// small-object workers 鈥?ModePauseOnProgress blocks workers, so only
	// 2 can be downloading at a time).
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
		t.Logf("downloading objects: %d (need >= 2)", downloading)
		return downloading >= 2
	}, 15*time.Second, 100*time.Millisecond, "expected at least 2 downloading objects")
	t.Logf("found %d downloading objects", downloading)

	// Concurrently cancel and force-download.
	// Make downloads complete quickly so CancelTask and retries race.
	// Switch downloader to always-success mode for the race.
	mgr.setDownloader(mockdl.New(mockdl.ModeAlwaysSuccess))

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

	// Wait for activeDownloads to settle (all non-negative).
	assert.MustEventually(t, func() bool {
		mgr.mu.Lock()
		defer mgr.mu.Unlock()
		for _, count := range mgr.activeDownloads {
			if count < 0 {
				return false
			}
		}
		return true
	}, 5*time.Second, 100*time.Millisecond, "expected activeDownloads to be non-negative for all tasks")

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
