// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
)

func TestManager_SetTaskConfig_NewFields(t *testing.T) {
	t.Parallel()

	trueVal := true
	falseVal := false

	tests := []struct {
		name            string
		scrapeEnabled   *bool
		downloadEnabled *bool
		saveSubDir      string
		wantScrape      *bool
		wantDownload    *bool
		wantSaveSubDir  string
	}{
		{
			name:            "scrape_enabled_true",
			scrapeEnabled:   &trueVal,
			downloadEnabled: nil,
			saveSubDir:      "",
			wantScrape:      &trueVal,
			wantDownload:    nil,
			wantSaveSubDir:  "",
		},
		{
			name:            "download_enabled_false",
			scrapeEnabled:   nil,
			downloadEnabled: &falseVal,
			saveSubDir:      "",
			wantScrape:      nil,
			wantDownload:    &falseVal,
			wantSaveSubDir:  "",
		},
		{
			name:            "save_sub_dir_set",
			scrapeEnabled:   nil,
			downloadEnabled: nil,
			saveSubDir:      "videos",
			wantScrape:      nil,
			wantDownload:    nil,
			wantSaveSubDir:  "videos",
		},
		{
			name:            "all_fields_set",
			scrapeEnabled:   &trueVal,
			downloadEnabled: &falseVal,
			saveSubDir:      "videos/sub",
			wantScrape:      &trueVal,
			wantDownload:    &falseVal,
			wantSaveSubDir:  "videos/sub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Server: config.Server{
					WorkDir: t.TempDir(),
				},
				Downloader: config.Downloader{
					GlobalConcurrent: 5,
				},
				Tasks: []config.Task{
					{
						ID:   "test-task",
						Type: "mock",
						Extra: map[string]any{
							"max_concurrent":   2,
							"refresh_interval": 3600,
						},
					},
				},
			}
			cfg.ValidateAndClamp()
			m := NewManager(cfg)

			// Register a mock task so SetTaskConfig can find it
			m.tasks.Store("test-task", &mockTask{id: "test-task", typ: "mock"})

			concurrency := 3
			refreshInterval := 600
			result, err := m.SetTaskConfig("test-task", &concurrency, &refreshInterval, tt.scrapeEnabled, tt.downloadEnabled, tt.saveSubDir, &AuditInfo{
				Author:  "test",
				Source:  "unit_test",
				Message: "test config update",
			})
			if err != nil {
				t.Fatalf("SetTaskConfig() error = %v", err)
			}

			// Verify result map contains new field keys
			if _, ok := result["scrape_enabled"]; !ok && tt.scrapeEnabled != nil {
				t.Errorf("result should contain scrape_enabled")
			}
			if _, ok := result["download_enabled"]; !ok && tt.downloadEnabled != nil {
				t.Errorf("result should contain download_enabled")
			}
			if _, ok := result["save_sub_dir"]; !ok && tt.saveSubDir != "" {
				t.Errorf("result should contain save_sub_dir")
			}

			// Verify config was persisted
			curCfg := m.GetConfig()
			var updatedTask *config.Task
			for i := range curCfg.Tasks {
				if curCfg.Tasks[i].ID == "test-task" {
					updatedTask = &curCfg.Tasks[i]
					break
				}
			}
			if updatedTask == nil {
				t.Fatal("test-task not found in config after update")
			}

			// Check ScrapeEnabled
			if tt.wantScrape != nil {
				if updatedTask.ScrapeEnabled == nil {
					t.Errorf("ScrapeEnabled = nil, want %v", *tt.wantScrape)
				} else if *updatedTask.ScrapeEnabled != *tt.wantScrape {
					t.Errorf("ScrapeEnabled = %v, want %v", *updatedTask.ScrapeEnabled, *tt.wantScrape)
				}
			} else {
				if updatedTask.ScrapeEnabled != nil {
					t.Errorf("ScrapeEnabled = %v, want nil", *updatedTask.ScrapeEnabled)
				}
			}

			// Check DownloadEnabled
			if tt.wantDownload != nil {
				if updatedTask.DownloadEnabled == nil {
					t.Errorf("DownloadEnabled = nil, want %v", *tt.wantDownload)
				} else if *updatedTask.DownloadEnabled != *tt.wantDownload {
					t.Errorf("DownloadEnabled = %v, want %v", *updatedTask.DownloadEnabled, *tt.wantDownload)
				}
			} else {
				if updatedTask.DownloadEnabled != nil {
					t.Errorf("DownloadEnabled = %v, want nil", *updatedTask.DownloadEnabled)
				}
			}

			// Check SaveSubDir
			if updatedTask.SaveSubDir != tt.wantSaveSubDir {
				t.Errorf("SaveSubDir = %q, want %q", updatedTask.SaveSubDir, tt.wantSaveSubDir)
			}
		})
	}
}
