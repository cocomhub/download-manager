// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mockdl

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
	"github.com/cocomhub/download-manager/task"
	mocktask "github.com/cocomhub/download-manager/task/mock"
)

// MockTaskConfig holds the configuration for creating a mock task programmatically.
type MockTaskConfig struct {
	TaskID      string
	Rules       []mocktask.MockRule
	Behavior    mocktask.MockBehavior
	Objects     []*model.DownloadObject // Directly injected objects (alternative to Rules)
	Storage     core.Storage
	Concurrency int
	SaveDir     string
}

// NewTask creates a mock task programmatically for testing, without needing a YAML config.
// The task is registered in the factory (triggered by import) and fully functional with the Manager.
func NewTask(t testing.TB, cfg MockTaskConfig) core.Task {
	t.Helper()

	if cfg.TaskID == "" {
		cfg.TaskID = "mock-test"
	}
	if cfg.SaveDir == "" {
		cfg.SaveDir = t.TempDir()
	}
	if cfg.Storage == nil {
		s, err := storage.NewMemoryStorage(nil)
		if err != nil {
			t.Fatalf("NewMemoryStorage: %v", err)
		}
		cfg.Storage = s
	}

	extra := map[string]any{}

	// Serialize rules.
	if len(cfg.Rules) > 0 {
		var rulesRaw []any
		for _, r := range cfg.Rules {
			rm := map[string]any{
				"url_template": r.URLTemplate,
				"count":        r.Count,
				"file_size":    r.FileSize,
				"status":       r.Status,
			}
			if len(r.Slugs) > 0 {
				slugsRaw := make([]any, len(r.Slugs))
				for i, s := range r.Slugs {
					slugsRaw[i] = s
				}
				rm["slugs"] = slugsRaw
			}
			if r.InitialProgress > 0 {
				rm["initial_progress"] = r.InitialProgress
			}
			if len(r.Metadata) > 0 {
				metaRaw := make(map[string]any, len(r.Metadata))
				for k, v := range r.Metadata {
					metaRaw[k] = v
				}
				rm["metadata"] = metaRaw
			}
			rulesRaw = append(rulesRaw, rm)
		}
		extra["mock_rules"] = rulesRaw
	}

	// Serialize behavior.
	b := map[string]any{
		"mode": cfg.Behavior.Mode,
	}
	if cfg.Behavior.FailRate > 0 {
		b["fail_rate"] = cfg.Behavior.FailRate
	}
	if cfg.Behavior.DelayPerByte > 0 {
		b["delay_per_byte"] = cfg.Behavior.DelayPerByte
	}
	if len(cfg.Behavior.FailOnURLs) > 0 {
		urls := make([]any, len(cfg.Behavior.FailOnURLs))
		for i, u := range cfg.Behavior.FailOnURLs {
			urls[i] = u
		}
		b["fail_on_urls"] = urls
	}
	if len(cfg.Behavior.TimeoutOnURLs) > 0 {
		urls := make([]any, len(cfg.Behavior.TimeoutOnURLs))
		for i, u := range cfg.Behavior.TimeoutOnURLs {
			urls[i] = u
		}
		b["timeout_on_urls"] = urls
	}
	extra["mock_behavior"] = b

	if cfg.Concurrency > 0 {
		extra["max_concurrent"] = cfg.Concurrency
	}

	taskCfg := &config.Task{
		ID:      cfg.TaskID,
		Type:    "mock",
		SaveDir: cfg.SaveDir,
		Storage: config.StorageConfig{Type: "memory"},
		Extra:   extra,
	}

	tsk, err := task.NewTask(taskCfg, task.WithStore(cfg.Storage))
	if err != nil {
		t.Fatalf("NewTask(mock): %v", err)
	}

	// Pre-seed objects if provided directly.
	if len(cfg.Objects) > 0 {
		for _, obj := range cfg.Objects {
			obj.TaskID = cfg.TaskID
			_ = cfg.Storage.Update(obj)
		}
	}

	return tsk
}
