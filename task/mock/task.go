// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package mock implements a configurable mock task type for testing.
// It registers itself as "mock" in the task factory and generates
// DownloadObjects from rules defined in the task's extra config.
package mock

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/task"
)

func init() {
	task.Register("mock", newMockTask)
}

// Task is a mock task for testing. It generates DownloadObjects from
// configurable rules and supports a mock downloader behavior.
type Task struct {
	*task.BaseTask
	rules  []MockRule
	seeded atomic.Bool
}

// newMockTask is the factory function registered with the task system.
func newMockTask(cfg *config.Task, opts task.Options) (core.Task, error) {
	bt, err := task.NewBaseTask(cfg, opts)
	if err != nil {
		return nil, err
	}

	mockRules, err := parseMockRules(cfg.Extra)
	if err != nil {
		return nil, fmt.Errorf("mock task: %w", err)
	}

	return &Task{
		BaseTask: bt,
		rules:    mockRules,
	}, nil
}

// Type returns "mock".
func (t *Task) Type() string { return "mock" }

// GetDownloadObjects returns all non-terminal objects.
// On first call, it seeds the storage with objects generated from rules.
func (t *Task) GetDownloadObjects() ([]*model.DownloadObject, error) {
	if !t.seeded.Load() {
		if err := t.seedObjects(); err != nil {
			return nil, err
		}
	}

	// Return non-terminal objects.
	all := t.GetAllObjects(true)
	var pending []*model.DownloadObject
	for _, obj := range all {
		s := obj.GetStatus()
		if s != model.StatusCompleted && s != model.StatusCancelled {
			pending = append(pending, obj)
		}
	}
	return pending, nil
}

// Scrape implements core.ScrapeCap. If refresh_interval > 0, it generates
// a new batch of objects (simulating new content being discovered).
// If refresh_interval <= 0, it is a no-op.
func (t *Task) Scrape(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if t.RefreshInterval() <= 0 {
		return nil
	}

	// Generate a new batch with a unique offset suffix.
	allObjectCount := len(t.GetAllObjects(true))
	for _, rule := range t.rules {
		objects := rule.generateObjects(t.ID(), allObjectCount)
		for _, obj := range objects {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			t.PersistTaskObject(obj)
		}
	}
	return nil
}

// ResolveObject is a no-op for mock tasks since URLs are directly downloadable.
// It marks the object as resolved by setting Extra["files"] so that
// the Manager's processTask can proceed to enqueue it for download.
func (t *Task) ResolveObject(_ context.Context, obj *model.DownloadObject) error {
	if obj.Extra == nil {
		obj.Extra = make(map[string]any)
	}
	if _, ok := obj.Extra["files"]; !ok {
		obj.Extra["files"] = []map[string]string{{"url": obj.URL}}
	}
	return nil
}

// seedObjects generates DownloadObjects from rules and persists them.
func (t *Task) seedObjects() error {
	// If already seeded, skip (double-check pattern).
	if t.seeded.Load() {
		return nil
	}

	var urlOffset int
	allObjects := t.GetAllObjects(true)
	if len(allObjects) > 0 {
		// Calculate offset from existing objects.
		urlOffset = len(allObjects)
	}

	for _, rule := range t.rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("mock task: invalid rule: %w", err)
		}
		objects := rule.generateObjects(t.ID(), urlOffset)
		for _, obj := range objects {
			t.PersistTaskObject(obj)
			t.RememberRuntimeObject(obj, true)
		}
		urlOffset += len(objects)
	}

	// Set seeded AFTER all objects are persisted.
	t.seeded.Store(true)
	return nil
}

// compile-time interface checks.
var _ core.Task = (*Task)(nil)
var _ core.ScrapeCap = (*Task)(nil)
