// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"maps"
	"sync"

	"github.com/cocomhub/download-manager/model"
)

// URLStateRegistry 提供基于 URL 的全局对象状态共享
type URLStateRegistry struct {
	mu      sync.RWMutex
	objects map[string]*model.DownloadObject
	owners  map[string]map[string]struct{}
	subs    []chan *model.DownloadObject
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
	defer r.mu.RUnlock()
	if obj, ok := r.objects[url]; ok {
		return cloneObject(obj), nil
	}
	return nil, nil
}

func (r *URLStateRegistry) Update(obj *model.DownloadObject) error {
	if obj == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.objects[obj.URL] = cloneObject(obj)
	if obj.TaskID != "" {
		owners := r.owners[obj.URL]
		if owners == nil {
			owners = make(map[string]struct{})
			r.owners[obj.URL] = owners
		}
		owners[obj.TaskID] = struct{}{}
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
	defer r.mu.RUnlock()
	if s := r.owners[url]; s != nil {
		return len(s)
	}
	return 0
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
