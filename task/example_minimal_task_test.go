// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"sync"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"
)

type fakeStorage struct {
	mu  sync.RWMutex
	mem map[string]*model.DownloadObject
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{mem: make(map[string]*model.DownloadObject)}
}

func (f *fakeStorage) Get(id string) (*model.DownloadObject, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.mem[id], nil
}

func (f *fakeStorage) Update(obj *model.DownloadObject) error {
	if obj == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mem[obj.URL] = &model.DownloadObject{
		TaskID:   obj.TaskID,
		URL:      obj.URL,
		SavePath: obj.SavePath,
		Metadata: obj.Metadata,
		Extra:    obj.Extra,
		Status:   obj.Status,
		Progress: obj.Progress,
	}
	return nil
}

func (f *fakeStorage) Delete(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.mem, id)
	return nil
}

func (f *fakeStorage) Search(filter any) ([]*model.DownloadObject, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var out []*model.DownloadObject
	for _, v := range f.mem {
		out = append(out, v)
	}
	return out, nil
}

type fakeRegistry struct {
	mu  sync.RWMutex
	mem map[string]*model.DownloadObject
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{mem: make(map[string]*model.DownloadObject)}
}

func (r *fakeRegistry) Get(url string) (*model.DownloadObject, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mem[url], nil
}

func (r *fakeRegistry) Update(obj *model.DownloadObject) error {
	if obj == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mem[obj.URL] = &model.DownloadObject{
		TaskID:   obj.TaskID,
		URL:      obj.URL,
		SavePath: obj.SavePath,
		Metadata: obj.Metadata,
		Extra:    obj.Extra,
		Status:   obj.Status,
		Progress: obj.Progress,
	}
	return nil
}

func (r *fakeRegistry) Delete(url string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.mem, url)
	return nil
}

func TestExampleMinimalTask_UpdateStatusAndClose(t *testing.T) {
	fs := newFakeStorage()
	fr := newFakeRegistry()

	cfg := config.Task{
		ID:      "example-1",
		Type:    "example_minimal",
		SaveDir: "/tmp/save",
	}
	task := NewExampleMinimalTask(cfg, fs)
	task.SetSharedRegistry(fr)

	obj := &model.DownloadObject{
		TaskID:   cfg.ID,
		URL:      "http://example.com/file.dat",
		SavePath: "/tmp/save/file.dat",
		Status:   "",
	}

	if err := task.UpdateStatus(obj, dlcore.StatusPending, nil); err != nil {
		t.Fatalf("UpdateStatus error: %v", err)
	}

	gotS, _ := fs.Get(obj.URL)
	if gotS == nil {
		t.Fatalf("fakeStorage missing object")
	}
	if gotS.Status != dlcore.StatusPending {
		t.Fatalf("fakeStorage status = %s, want %s", gotS.Status, dlcore.StatusPending)
	}

	gotR, _ := fr.Get(obj.URL)
	if gotR == nil {
		t.Fatalf("fakeRegistry missing object")
	}
	if gotR.Status != dlcore.StatusPending {
		t.Fatalf("fakeRegistry status = %s, want %s", gotR.Status, dlcore.StatusPending)
	}

	_ = task.Close()
}
