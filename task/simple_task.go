// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"
)

type SimpleTask struct {
	BaseTask
}

// Ensure SimpleTask implements core.Task
var _ core.Task = &SimpleTask{}

func NewSimpleTask(id string, urls []string, saveDir string, store core.Storage) *SimpleTask {
	t := &SimpleTask{
		BaseTask: NewBaseTask(id, saveDir, store),
	}

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
			if storedObj, err := store.Get(u); err == nil && storedObj != nil {
				obj.Status = storedObj.Status
				obj.Metadata = storedObj.Metadata
				obj.Extra = storedObj.Extra
				t.ResetZombieState(obj)
			}
		}

		t.objects = append(t.objects, obj)
	}

	return t
}

func (t *SimpleTask) Type() string {
	return "simple_url_list"
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
