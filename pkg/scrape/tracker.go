// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package scrape

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Tracker persists the full-scan success marker and progress (for interrupted scans).
type Tracker interface {
	IsFullSucceeded(taskID string) bool
	MarkFullSucceeded(taskID string) error
	DeleteFullSuccess(taskID string) error
	LoadProgress(taskID string) (ProgressInfo, bool)
	SaveProgress(taskID string, info ProgressInfo) error
	ClearProgress(taskID string) error
}

// FileTracker implements Tracker using JSON files under a root directory.
type FileTracker struct {
	root string
	mu   sync.Mutex
}

func NewFileTracker(rootDir string) *FileTracker {
	return &FileTracker{root: rootDir}
}

func (t *FileTracker) succPath(taskID string) string {
	return filepath.Join(t.root, "full_succ", taskID+".json")
}

func (t *FileTracker) progressPath(taskID string) string {
	return filepath.Join(t.root, "full_progress", taskID+".json")
}

func (t *FileTracker) IsFullSucceeded(taskID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, err := os.Stat(t.succPath(taskID))
	return err == nil
}

func (t *FileTracker) MarkFullSucceeded(taskID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	dir := filepath.Dir(t.succPath(taskID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(t.succPath(taskID))
	if err != nil {
		return err
	}
	return f.Close()
}

func (t *FileTracker) DeleteFullSuccess(taskID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := os.Remove(t.succPath(taskID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (t *FileTracker) LoadProgress(taskID string) (ProgressInfo, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	data, err := os.ReadFile(t.progressPath(taskID))
	if err != nil {
		return ProgressInfo{}, false
	}
	var info ProgressInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return ProgressInfo{}, false
	}
	return info, true
}

func (t *FileTracker) SaveProgress(taskID string, info ProgressInfo) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	dir := filepath.Dir(t.progressPath(taskID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(t.progressPath(taskID), data, 0644)
}

func (t *FileTracker) ClearProgress(taskID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := os.Remove(t.progressPath(taskID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
