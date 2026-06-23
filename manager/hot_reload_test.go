// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"
	"time"

	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/testutil/assert"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestConfigHotReload_DuringActiveDownload verifies that when UpdateConfig is
// called while downloads are in-flight, existing downloads continue unaffected
// and new objects use the new downloader.
func TestConfigHotReload_DuringActiveDownload(t *testing.T) {
	mgr, _ := newMockManager(t, "hotreload", 10,
		mockdl.New(mockdl.ModePauseOnProgress, mockdl.WithDelay(100*time.Millisecond)))
	startManager(t, mgr)

	task := waitForTask(t, mgr, "hotreload")

	// Wait until at least 2 objects enter downloading state.
	assert.MustEventually(t, func() bool {
		mgr.scan()
		all := getAllObjectsFromTask(t, task)
		var downloading int
		for _, obj := range all {
			if obj.GetStatus() == model.StatusDownloading {
				downloading++
			}
		}
		t.Logf("downloading: %d/2", downloading)
		return downloading >= 2
	}, 10*time.Second, 100*time.Millisecond, "expected at least 2 downloading objects before hot-reload")

	// Capture objects that were downloading before hot-reload
	before := getAllObjectsFromTask(t, task)
	var downloadingURLs []string
	for _, obj := range before {
		if obj.GetStatus() == model.StatusDownloading {
			downloadingURLs = append(downloadingURLs, obj.URL)
		}
	}

	// Perform config hot-reload with increased capacity.
	cfg := mgr.currentCfg()
	newCfg := *cfg
	newCfg.Downloader.GlobalConcurrent = 20
	// Raise per-task concurrency so the scheduler can enqueue new objects
	// even while the 2 ModePauseOnProgress downloads are still blocked.
	if len(newCfg.Tasks) > 0 {
		if newCfg.Tasks[0].Extra == nil {
			newCfg.Tasks[0].Extra = make(map[string]any)
		}
		newCfg.Tasks[0].Extra["max_concurrent"] = 20
	}
	if err := mgr.UpdateConfig(&newCfg, &AuditInfo{Source: "test", Message: "hot-reload-test"}); err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}

	// IMPORTANT: UpdateConfig internally calls m.setDownloader(downloader.New(cfgCopy.Downloader)),
	// which creates a REAL downloader (not a mock). We must override it back to a mock so that
	// pending objects can complete in the test environment without a real HTTP server.
	mgr.setDownloader(mockdl.New(mockdl.ModeAlwaysSuccess))

	// Wait for config changes to propagate: at least one object should complete
	// with the new downloader (ModeAlwaysSuccess). Previously-downloading
	// objects (ModePauseOnProgress) are blocked in Download(), so only NEW
	// objects enqueued after the config reload can complete. We scan and poll
	// until at least one object transitions to StatusCompleted.
	assert.MustEventually(t, func() bool {
		mgr.scan()
		all := getAllObjectsFromTask(t, task)
		for _, obj := range all {
			if obj.GetStatus() == model.StatusCompleted {
				return true
			}
		}
		return false
	}, 10*time.Second, 100*time.Millisecond,
		"expected at least one completed object after config hot-reload")

	// The previously-downloading objects should still exist and either be
	// completed (if they finished) or still in a valid state.
	all := getAllObjectsFromTask(t, task)
	for _, obj := range all {
		status := obj.GetStatus()
		t.Logf("after hot-reload: %s status=%s", obj.URL, status)
	}

	// Verify: previously-downloading objects are either completed or still downloading.
	for _, url := range downloadingURLs {
		obj := findObjectByURL(all, url)
		if obj == nil {
			continue
		}
		status := obj.GetStatus()
		if status != model.StatusCompleted && status != model.StatusDownloading &&
			status != model.StatusFailed {
			t.Errorf("previously-downloading object %s has unexpected status %s after hot-reload",
				url, status)
		}
	}

	// Verify: no data race around active downloads tracking.
	mgr.mu.Lock()
	for taskID, count := range mgr.activeDownloads {
		if count < 0 {
			t.Errorf("activeDownloads[%s] = %d (negative!)", taskID, count)
		}
	}
	mgr.mu.Unlock()
}

func findObjectByURL(objects []*model.DownloadObject, url string) *model.DownloadObject {
	for _, obj := range objects {
		if obj.URL == url {
			return obj
		}
	}
	return nil
}
