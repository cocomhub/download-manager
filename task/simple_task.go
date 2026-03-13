// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"
)

type SimpleTask struct {
	id      string
	urls    []string
	saveDir string
	objects []*model.DownloadObject
	store   core.Storage
	shared  core.SharedRegistry
	mu      sync.Mutex
}

// Ensure SimpleTask implements core.Task
var _ core.Task = &SimpleTask{}

func NewSimpleTask(id string, urls []string, saveDir string, store core.Storage) *SimpleTask {
	t := &SimpleTask{
		id:      id,
		urls:    urls,
		saveDir: saveDir,
		objects: make([]*model.DownloadObject, 0),
		store:   store,
	}

	// 1. Initialize potential objects from URLs (Source of Truth for "What to download")
	// 2. Check Storage for "What has been done" or "Current status"

	usedNames := make(map[string]bool)
	for i, u := range urls {
		filename := filepath.Base(u)
		if filename == "." || filename == "/" {
			filename = fmt.Sprintf("file_%d.dat", i)
		}

		// Handle duplicates
		originalName := filename
		counter := 1
		for usedNames[filename] {
			ext := filepath.Ext(originalName)
			name := strings.TrimSuffix(originalName, ext)
			filename = fmt.Sprintf("%s_%d%s", name, counter, ext)
			counter++
		}
		usedNames[filename] = true

		obj := &model.DownloadObject{
			TaskID:   id,
			URL:      u,
			SavePath: filepath.Join(saveDir, filename),
			Status:   dlcore.StatusPending,
		}

		// Check storage for this object
		if store != nil {
			// Try to get status by ID (using URL as ID for now)
			if storedObj, err := store.Get(u); err == nil && storedObj != nil {
				// Use stored status and metadata
				obj.Status = storedObj.Status
				obj.Metadata = storedObj.Metadata
				obj.Extra = storedObj.Extra

				// Fix "Zombie" downloading state
				// If we just started and storage says "downloading", it means it crashed.
				// Reset to pending.
				if obj.Status == dlcore.StatusDownloading {
					slog.Warn("Found zombie downloading state, resetting to pending", "task_id", id, "url", u)
					obj.Status = dlcore.StatusPending
					// We should probably sync this reset back to storage immediately or lazily
					// Let's sync immediately to be safe
					if err := store.Update(obj); err != nil {
						slog.Error("Failed to reset zombie state", "task_id", id, "error", err)
					}
				}
			}
		}

		t.objects = append(t.objects, obj)
	}

	return t
}

func (t *SimpleTask) ID() string {
	return t.id
}

func (t *SimpleTask) Type() string {
	return "simple_url_list"
}

func (t *SimpleTask) GetStorage() core.Storage {
	return t.store
}

func (t *SimpleTask) SetSharedRegistry(reg core.SharedRegistry) {
	t.shared = reg
}

func (t *SimpleTask) Close() error {
	// Flush storage if supported
	if t.store != nil {
		if flusher, ok := t.store.(interface{ ForceFlush() error }); ok {
			return flusher.ForceFlush()
		}
	}
	return nil
}

func (t *SimpleTask) GetDownloadHeaders() map[string]string {
	return map[string]string{}
}

func (t *SimpleTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Sync from shared registry if available
	if t.shared != nil {
		for _, obj := range t.objects {
			if storedObj, err := t.shared.Get(obj.URL); err == nil && storedObj != nil {
				*obj = *storedObj
			}
		}
	}

	var pending []*model.DownloadObject
	for _, obj := range t.objects {
		if obj.Status == dlcore.StatusPending || obj.Status == dlcore.StatusFailed {
			pending = append(pending, obj)
		}
	}
	return pending, nil
}

func (t *SimpleTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	obj.Status = status

	// Print log
	if err != nil {
		slog.Error("Object failed", "task_id", t.id, "url", obj.URL, "error", err)
	} else {
		slog.Info("Object status updated", "task_id", t.id, "url", obj.URL, "status", status)
	}

	// Update storage
	if t.store != nil {
		if storeErr := t.store.Update(obj); storeErr != nil {
			slog.Error("Failed to update storage", "task_id", t.id, "error", storeErr)
		}
	}
	// Update shared registry
	if t.shared != nil {
		_ = t.shared.Update(obj)
	}

	return nil
}

// New helper for API
func (t *SimpleTask) GetAllObjects() []*model.DownloadObject {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Return copy to be safe? Or just slice.
	// Slice is reference to underlying array, but objects are pointers too.
	// For JSON serialization this should be fine as long as no concurrent modification happens during marshal.
	return t.objects
}
