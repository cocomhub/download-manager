// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package urllist

import (
	"sync"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"
	"github.com/cocomhub/download-manager/task"
)

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
		Status:   obj.GetStatus(),
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

func TestTask_UpdateStatusAndClose(t *testing.T) {
	task, err := task.NewTask(&config.Task{
		ID:      "example1",
		Type:    TaskType,
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{
			Type:   "memory",
			Config: nil,
		},
	})
	if err != nil {
		t.Fatalf("new task err: %s", err)
	}

	fr := newFakeRegistry()
	task.(*Task).SetSharedRegistry(fr)

	obj := &model.DownloadObject{
		TaskID:   task.ID(),
		URL:      "http://example.com/file.dat",
		SavePath: "/tmp/save/file.dat",
		Status:   "",
	}

	if err := task.UpdateStatus(obj, dlcore.StatusPending, nil); err != nil {
		t.Fatalf("UpdateStatus error: %v", err)
	}

	gotS, _ := task.Storage().Get(obj.URL)
	if gotS == nil {
		t.Fatalf("fakeStorage missing object")
	}
	if gotS.GetStatus() != dlcore.StatusPending {
		t.Fatalf("fakeStorage status = %s, want %s", gotS.GetStatus(), dlcore.StatusPending)
	}

	gotR, _ := fr.Get(obj.URL)
	if gotR == nil {
		t.Fatalf("fakeRegistry missing object")
	}
	if gotR.GetStatus() != dlcore.StatusPending {
		t.Fatalf("fakeRegistry status = %s, want %s", gotR.GetStatus(), dlcore.StatusPending)
	}

	_ = task.Close()
}
