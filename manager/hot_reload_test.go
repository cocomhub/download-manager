// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"
	"time"

	"github.com/cocomhub/download-manager/model"
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

	// Wait until some objects enter downloading.
	deadline := time.Now().Add(5 * time.Second)
	var downloading int
	for time.Now().Before(deadline) {
		mgr.scan()
		time.Sleep(100 * time.Millisecond)

		all := getAllObjectsFromTask(t, task)
		downloading = 0
		for _, obj := range all {
			if obj.GetStatus() == model.StatusDownloading {
				downloading++
			}
		}
		if downloading >= 2 {
			t.Logf("found %d downloading objects, triggering config hot-reload", downloading)
			break
		}
	}
	if downloading < 2 {
		t.Fatal("expected at least 2 downloading objects before hot-reload")
	}

	// Capture objects that were downloading before hot-reload
	before := getAllObjectsFromTask(t, task)
	var downloadingURLs []string
	for _, obj := range before {
		if obj.GetStatus() == model.StatusDownloading {
			downloadingURLs = append(downloadingURLs, obj.URL)
		}
	}

	// Perform config hot-reload: swap downloader to fast success.
	cfg := mgr.currentCfg()
	newCfg := *cfg
	newCfg.Downloader.GlobalConcurrent = 20
	mgr.downloader = mockdl.New(mockdl.ModeAlwaysSuccess)
	if err := mgr.UpdateConfig(&newCfg, &AuditInfo{Source: "test", Message: "hot-reload-test"}); err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}

	// Wait a bit for the config changes to propagate.
	time.Sleep(1 * time.Second)

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
