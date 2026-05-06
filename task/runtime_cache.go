// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"
)

const (
	runtimeObjectLimit         = 256
	runtimeTerminalObjectLimit = 32
)

func runtimeObjectExists(objects []*model.DownloadObject, url string) bool {
	for _, obj := range objects {
		if obj != nil && obj.URL == url {
			return true
		}
	}
	return false
}

func storageExistenceMap(store core.Storage, runtimeObjects []*model.DownloadObject, urls []string) map[string]bool {
	result := make(map[string]bool, len(urls))
	missing := make([]string, 0, len(urls))
	for _, url := range urls {
		if url == "" {
			continue
		}
		if _, ok := result[url]; ok {
			continue
		}
		if runtimeObjectExists(runtimeObjects, url) {
			result[url] = true
			continue
		}
		result[url] = false
		missing = append(missing, url)
	}
	if len(missing) == 0 || store == nil {
		return result
	}
	exists, err := store.Exists(missing)
	if err != nil {
		return result
	}
	for _, url := range missing {
		result[url] = exists[url]
	}
	return result
}

func persistTaskObject(store core.Storage, shared core.SharedRegistry, obj *model.DownloadObject) {
	if obj == nil {
		return
	}
	if store != nil {
		_ = store.Update(obj)
	}
	if shared != nil {
		_ = shared.Update(obj)
	}
}

func upsertRuntimeObject(objects []*model.DownloadObject, obj *model.DownloadObject) []*model.DownloadObject {
	if obj == nil {
		return objects
	}
	for idx, existing := range objects {
		if existing != nil && existing.URL == obj.URL {
			objects[idx] = obj
			if idx == 0 {
				return pruneRuntimeObjects(objects)
			}
			copy(objects[1:idx+1], objects[0:idx])
			objects[0] = obj
			return pruneRuntimeObjects(objects)
		}
	}
	objects = append([]*model.DownloadObject{obj}, objects...)
	return pruneRuntimeObjects(objects)
}

func pruneRuntimeObjects(objects []*model.DownloadObject) []*model.DownloadObject {
	if len(objects) == 0 {
		return objects
	}
	capHint := min(len(objects), runtimeObjectLimit)
	kept := make([]*model.DownloadObject, 0, capHint)
	terminalCount := 0
	for _, obj := range objects {
		if obj == nil {
			continue
		}
		terminal := obj.Status == dlcore.StatusCompleted || obj.Status == dlcore.StatusCancelled
		if terminal {
			if terminalCount >= runtimeTerminalObjectLimit {
				continue
			}
			terminalCount++
		}
		kept = append(kept, obj)
		if len(kept) >= runtimeObjectLimit {
			break
		}
	}
	return kept
}

func rememberRuntimeURLs(objects []*model.DownloadObject) map[string]bool {
	known := make(map[string]bool, len(objects))
	for _, obj := range objects {
		if obj == nil || obj.URL == "" {
			continue
		}
		known[obj.URL] = true
	}
	return known
}
