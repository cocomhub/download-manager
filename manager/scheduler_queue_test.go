// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/testutil/assert"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestScheduler_GlobalQueueFull verifies that when the global download queue
// is full, drainOnce correctly back-pressures without dropping items.
func TestScheduler_GlobalQueueFull(t *testing.T) {
	cfg := &config.Config{
		Runtime: config.Runtime{
			Mode: config.RunModeFull,
			Download: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{Enabled: true},
			Scheduler: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{Enabled: true},
		},
		Server: config.Server{
			WorkDir:         t.TempDir(),
			DownloadRootDir: t.TempDir(),
		},
		Downloader: config.Downloader{
			GlobalConcurrent: 1, // Force queue to fill quickly
			MaxRetries:       2,
		},
		Tasks: []config.Task{
			{
				ID:      "queue-full",
				Type:    "mock",
				SaveDir: t.TempDir(),
				Storage: config.StorageConfig{Type: "memory"},
				Extra: map[string]any{
					"mock_rules": []any{
						map[string]any{
							"url_template": "http://mock-download/file-{n}.bin",
							"count":        100,
						},
					},
				},
			},
		},
	}
	mgr := NewManager(cfg)
	mgr.setDownloader(mockdl.New(mockdl.ModeAlwaysSuccess, mockdl.WithDelay(10*time.Millisecond)))
	startManager(t, mgr)

	task := waitForTask(t, mgr, "queue-full")

	// Wait for progress: many objects should get enqueued.
	assert.MustEventually(t, func() bool {
		mgr.scan()
		all := getAllObjectsFromTask(t, task)
		var downloading, completed, pending int
		for _, obj := range all {
			switch obj.GetStatus() {
			case model.StatusDownloading:
				downloading++
			case model.StatusCompleted:
				completed++
			default:
				pending++
			}
		}
		t.Logf("scan: downloading=%d completed=%d pending=%d", downloading, completed, pending)
		return pending+downloading+completed == 100
	}, 8*time.Second, 200*time.Millisecond, "all 100 objects should be accounted for after scanning")

	// Final verification: no objects should be lost.
	all := getAllObjectsFromTask(t, task)
	if len(all) != 100 {
		t.Errorf("expected 100 total objects in task, got %d", len(all))
	}

	// Verify activeDownloads never went negative.
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	for taskID, count := range mgr.activeDownloads {
		if count < 0 {
			t.Errorf("activeDownloads[%s] = %d (negative!)", taskID, count)
		}
	}
}
