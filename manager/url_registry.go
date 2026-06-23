// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"maps"
	"sync"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

type registeredStorage struct {
	taskID string
	store  core.Storage
}

// URLStateRegistry 鎻愪緵鍩轰簬 URL 鐨勫叏灞€瀵硅薄鐘舵€佸叡浜?type URLStateRegistry struct {
	mu      sync.RWMutex
	objects map[string]*model.DownloadObject
	owners  map[string]map[string]struct{}
	subs    []chan *model.DownloadObject
	stores  []registeredStorage
}

func NewURLStateRegistry() *URLStateRegistry {
	return &URLStateRegistry{
		objects: make(map[string]*model.DownloadObject),
		owners:  make(map[string]map[string]struct{}),
	}
}

func cloneObject(src *model.DownloadObject) *model.DownloadObject {
	if src == nil {
		return nil
	}
	// Lock the source object to protect against concurrent writes to Extra/Metadata.
	// Direct field reads are safe under RLock; do NOT call GetStatus/GetProgress here
	// because Go's RWMutex is not reentrant and would deadlock.
	src.RLock()
	defer src.RUnlock()
	dst := &model.DownloadObject{
		TaskID:   src.TaskID,
		URL:      src.URL,
		SavePath: src.SavePath,
		Status:   src.Status,
		Progress: src.Progress,
	}
	if src.Metadata != nil {
		dst.Metadata = make(map[string]string, len(src.Metadata))
		maps.Copy(dst.Metadata, src.Metadata)
	}
	if src.Extra != nil {
		dst.Extra = make(map[string]any, len(src.Extra))
		maps.Copy(dst.Extra, src.Extra)
	}
	return dst
}

func (r *URLStateRegistry) Get(url string) (*model.DownloadObject, error) {
	r.mu.RLock()
	if obj, ok := r.objects[url]; ok {
		r.mu.RUnlock()
		return cloneObject(obj), nil
	}
	stores := append([]registeredStorage(nil), r.stores...)
	r.mu.RUnlock()

	var best *model.DownloadObject
	owners := make(map[string]struct{})
	for _, entry := range stores {
		if entry.store == nil {
			continue
		}
		obj, err := entry.store.Get(url)
		if err != nil || obj == nil {
			continue
		}
		if obj.GetStatus() != model.StatusPending {
			owners[ownerID(entry.taskID, obj)] = struct{}{}
		}
		if betterSharedObject(obj, best) {
			best = obj
		}
	}
	if best == nil {
		return nil, nil
	}

	r.mu.Lock()
	if existing, ok := r.objects[url]; ok {
		for owner := range owners {
			r.addOwnerLocked(url, owner)
		}
		r.mu.Unlock()
		return cloneObject(existing), nil
	}
	r.objects[url] = cloneObject(best)
	for owner := range owners {
		r.addOwnerLocked(url, owner)
	}
	cached := cloneObject(r.objects[url])
	r.mu.Unlock()
	return cached, nil
}

func (r *URLStateRegistry) Update(obj *model.DownloadObject) error {
	if obj == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.objects[obj.URL] = cloneObject(obj)
	if obj.GetStatus() != model.StatusPending {
		r.addOwnerLocked(obj.URL, stringsTrim(obj.TaskID))
	}
	for _, ch := range r.subs {
		select {
		case ch <- r.objects[obj.URL]:
		default:
		}
	}
	return nil
}

func (r *URLStateRegistry) Delete(url string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.objects, url)
	delete(r.owners, url)
	for _, ch := range r.subs {
		select {
		case ch <- &model.DownloadObject{URL: url, Status: model.StatusFailed}:
		default:
		}
	}
	return nil
}

func (r *URLStateRegistry) Owners(url string) int {
	r.mu.RLock()
	if s := r.owners[url]; s != nil {
		r.mu.RUnlock()
		return len(s)
	}
	r.mu.RUnlock()
	_, _ = r.Get(url)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s := r.owners[url]; s != nil {
		return len(s)
	}
	return 0
}

func (r *URLStateRegistry) RegisterStorage(taskID string, store core.Storage) {
	if store == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.stores {
		if r.stores[i].taskID == taskID {
			r.stores[i].store = store
			return
		}
	}
	r.stores = append(r.stores, registeredStorage{taskID: taskID, store: store})
}

func (r *URLStateRegistry) Subscribe() <-chan *model.DownloadObject {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := make(chan *model.DownloadObject, 100)
	r.subs = append(r.subs, ch)
	return ch
}

func (r *URLStateRegistry) Unsubscribe(ch <-chan *model.DownloadObject) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.subs {
		if r.subs[i] == ch {
			close(r.subs[i])
			r.subs = append(r.subs[:i], r.subs[i+1:]...)
			break
		}
	}
}

func (r *URLStateRegistry) addOwnerLocked(url, taskID string) {
	if taskID == "" {
		return
	}
	owners := r.owners[url]
	if owners == nil {
		owners = make(map[string]struct{})
		r.owners[url] = owners
	}
	owners[taskID] = struct{}{}
}

func ownerID(taskID string, obj *model.DownloadObject) string {
	if trimmed := stringsTrim(taskID); trimmed != "" {
		return trimmed
	}
	if obj == nil {
		return ""
	}
	return stringsTrim(obj.TaskID)
}

func betterSharedObject(candidate, current *model.DownloadObject) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}
	left := sharedObjectScore(candidate)
	right := sharedObjectScore(current)
	if left != right {
		return left > right
	}
	return len(candidate.Metadata)+len(candidate.Extra) > len(current.Metadata)+len(current.Extra)
}

func sharedObjectScore(obj *model.DownloadObject) int {
	if obj == nil {
		return -1
	}
	score := 0
	switch obj.GetStatus() {
	case model.StatusCompleted:
		score += 40
	case model.StatusDownloading:
		score += 30
	case model.StatusFailed:
		score += 20
	case model.StatusPending:
		score += 10
	case model.StatusCancelled:
		score += 5
	}
	if _, ok := obj.Extra["files"]; ok {
		score += 5
	}
	score += len(obj.Metadata)
	score += len(obj.Extra)
	return score
}

func stringsTrim(value string) string {
	start := 0
	end := len(value)
	for start < end && (value[start] == ' ' || value[start] == '\t' || value[start] == '\n' || value[start] == '\r') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t' || value[end-1] == '\n' || value[end-1] == '\r') {
		end--
	}
	return value[start:end]
}
