// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package urllist

import (
	"sync"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
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

	if err := task.UpdateStatus(obj, model.StatusPending, nil); err != nil {
		t.Fatalf("UpdateStatus error: %v", err)
	}

	gotS, _ := task.Storage().Get(obj.URL)
	if gotS == nil {
		t.Fatalf("fakeStorage missing object")
	}
	if gotS.GetStatus() != model.StatusPending {
		t.Fatalf("fakeStorage status = %s, want %s", gotS.GetStatus(), model.StatusPending)
	}

	gotR, _ := fr.Get(obj.URL)
	if gotR == nil {
		t.Fatalf("fakeRegistry missing object")
	}
	if gotR.GetStatus() != model.StatusPending {
		t.Fatalf("fakeRegistry status = %s, want %s", gotR.GetStatus(), model.StatusPending)
	}

	_ = task.Close()
}

// TestInitDownloadObject_PreservesCloudDownload 验证「重启/重建对象」时保留下载项级
// 云端下载选项（否则 urllist 任务重启即静默丢失该标志）。
func TestInitDownloadObject_PreservesCloudDownload(t *testing.T) {
	t.Parallel()
	const u = "https://example.com/keep-cloud.dat"
	tk, err := task.NewTask(&config.Task{
		ID:      "cd-keep",
		Type:    TaskType,
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
		Extra:   map[string]any{"urls": []string{u}},
	})
	if err != nil {
		t.Fatalf("new task err: %s", err)
	}
	tt := tk.(*Task)
	objs := tt.GetAllObjects(true)
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}
	objs[0].SetCloudDownload(true)
	if err := tt.Storage().Update(objs[0]); err != nil {
		t.Fatalf("update: %s", err)
	}

	// 模拟重启：用存储中的既有对象重建
	if err := tt.initDownloadObject(u, 0, map[string]bool{}); err != nil {
		t.Fatalf("initDownloadObject: %s", err)
	}
	stored := tt.GetCachedObject(u)
	if stored == nil || !stored.IsCloudDownload() {
		t.Fatal("cloud_download 未在重建对象时保留")
	}
}

// TestSyncObjectOption_UpdatesRuntimeObject 验证运行时对象按 URL 同步「云端下载」选项。
func TestSyncObjectOption_UpdatesRuntimeObject(t *testing.T) {
	t.Parallel()
	const u = "https://example.com/sync-cloud.dat"
	tk, err := task.NewTask(&config.Task{
		ID:      "cd-sync",
		Type:    TaskType,
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
		Extra:   map[string]any{"urls": []string{u}},
	})
	if err != nil {
		t.Fatalf("new task err: %s", err)
	}
	tt := tk.(*Task)
	on := true
	if tt.SyncObjectOption("https://example.com/unknown", model.ObjectOption{CloudDownload: &on}) {
		t.Fatal("未知 URL 不应命中")
	}
	// 零值选项（nil 字段）= 不同步任何项，但仍应命中 URL
	if !tt.SyncObjectOption(u, model.ObjectOption{CloudDownload: &on}) {
		t.Fatal("已知 URL 应命中")
	}
	if !tt.SyncObjectOption(u, model.ObjectOption{}) {
		t.Fatal("已知 URL 应命中")
	}
	for _, o := range tt.GetAllObjects(true) {
		if o.URL == u && !o.IsCloudDownload() {
			t.Fatal("零值选项不应把已置位的字段改回")
		}
	}
	if !tt.SyncObjectOption(u, model.ObjectOption{CloudDownload: &on}) {
		t.Fatal("已知 URL 应命中")
	}
	for _, o := range tt.GetAllObjects(true) {
		if o.URL == u && !o.IsCloudDownload() {
			t.Fatal("运行时对象未同步 cloud_download")
		}
	}
}
