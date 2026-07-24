// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"embed"
	"sync"
)

// TaskUIAssets describes custom JS/CSS assets a task type may register
// to enhance its display in the Web UI (e.g. a comic reader for "mxs").
type TaskUIAssets struct {
	FS       embed.FS // embedded filesystem containing the assets
	JSPaths  []string // file paths relative to FS root (e.g. ["reader.js"])
	CSSPaths []string // CSS file paths relative to FS root
	Label    string   // button label shown in the UI, e.g. "阅读"
}

var (
	uiRegistry   sync.Map // taskType -> TaskUIAssets
	uiTypeListMu sync.RWMutex
	uiTypeList   []string // cached list of registered types
)

// RegisterTaskUI registers custom UI assets for the given task type.
// Typically called from an init() function inside the task's ui sub-package.
func RegisterTaskUI(taskType string, assets TaskUIAssets) {
	uiRegistry.Store(taskType, assets)
	uiTypeListMu.Lock()
	uiTypeList = append(uiTypeList, taskType)
	uiTypeListMu.Unlock()
}

// GetTaskUI returns the registered UI assets for a task type, if any.
func GetTaskUI(taskType string) (TaskUIAssets, bool) {
	v, ok := uiRegistry.Load(taskType)
	if !ok {
		return TaskUIAssets{}, false
	}
	assets, ok := v.(TaskUIAssets)
	if !ok {
		return TaskUIAssets{}, false
	}
	return assets, true
}

// ListRegisteredUI returns the task types that have registered UI assets.
func ListRegisteredUI() []string {
	uiTypeListMu.RLock()
	defer uiTypeListMu.RUnlock()
	out := make([]string, len(uiTypeList))
	copy(out, uiTypeList)
	return out
}
