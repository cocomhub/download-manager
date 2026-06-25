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

	fillDefaults(t, &cfg)

	extra := map[string]any{}
	if len(cfg.Rules) > 0 {
		extra["mock_rules"] = serializeMockRules(cfg.Rules)
	}
	extra["mock_behavior"] = serializeMockBehavior(cfg.Behavior)
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

	seedObjects(cfg.Objects, cfg.TaskID, cfg.Storage)

	return tsk
}

// fillDefaults sets default values for MockTaskConfig fields that are not set.
func fillDefaults(t testing.TB, cfg *MockTaskConfig) {
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
}

// serializeMockRules converts MockRule slices to the serialized []any format.
func serializeMockRules(rules []mocktask.MockRule) []any {
	rulesRaw := make([]any, len(rules))
	for i, r := range rules {
		rm := map[string]any{
			"url_template": r.URLTemplate,
			"count":        r.Count,
			"file_size":    r.FileSize,
			"status":       r.Status,
		}
		if len(r.Slugs) > 0 {
			slugsRaw := make([]any, len(r.Slugs))
			for j, s := range r.Slugs {
				slugsRaw[j] = s
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
		rulesRaw[i] = rm
	}
	return rulesRaw
}

// serializeMockBehavior converts a MockBehavior to a serialized map format.
func serializeMockBehavior(b mocktask.MockBehavior) map[string]any {
	m := map[string]any{
		"mode": b.Mode,
	}
	if b.FailRate > 0 {
		m["fail_rate"] = b.FailRate
	}
	if b.DelayPerByte > 0 {
		m["delay_per_byte"] = b.DelayPerByte
	}
	if len(b.FailOnURLs) > 0 {
		urls := make([]any, len(b.FailOnURLs))
		for i, u := range b.FailOnURLs {
			urls[i] = u
		}
		m["fail_on_urls"] = urls
	}
	if len(b.TimeoutOnURLs) > 0 {
		urls := make([]any, len(b.TimeoutOnURLs))
		for i, u := range b.TimeoutOnURLs {
			urls[i] = u
		}
		m["timeout_on_urls"] = urls
	}
	return m
}

// seedObjects pre-seeds DownloadObject entries into storage.
func seedObjects(objects []*model.DownloadObject, taskID string, store core.Storage) {
	for _, obj := range objects {
		obj.TaskID = taskID
		_ = store.Update(obj)
	}
}
