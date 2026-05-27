// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"unsafe"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
	"github.com/cocomhub/download-manager/task"
)

type loaderTestTask struct {
	t                *testing.T
	mgr              *Manager
	id               string
	typ              string
	store            core.Storage
	startErr         error
	checkBeforeStore bool
}

func (tt *loaderTestTask) ID() string { return tt.id }
func (tt *loaderTestTask) Type() string {
	if tt.typ == "" {
		return "test"
	}
	return tt.typ
}
func (tt *loaderTestTask) Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
func (tt *loaderTestTask) Storage() core.Storage { return tt.store }
func (tt *loaderTestTask) SetDownloader(core.Downloader) {
}
func (tt *loaderTestTask) GetDownloadHeaders() map[string]string { return map[string]string{} }
func (tt *loaderTestTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	return []*model.DownloadObject{}, nil
}
func (tt *loaderTestTask) UpdateStatus(*model.DownloadObject, string, error) error { return nil }
func (tt *loaderTestTask) Concurrency() int                                        { return 1 }
func (tt *loaderTestTask) SetConcurrency(int) error                                { return nil }
func (tt *loaderTestTask) RefreshInterval() int                                    { return 60 }
func (tt *loaderTestTask) SetRefreshInterval(int) error                            { return nil }
func (tt *loaderTestTask) Start() error {
	if tt.checkBeforeStore && tt.mgr != nil && tt.t != nil {
		if _, ok := tt.mgr.tasks.Load(tt.id); ok {
			tt.t.Fatalf("task should not be stored before Start returns success")
		}
		if hasRegisteredStore(tt.mgr.urlRegistry, tt.id) {
			tt.t.Fatalf("storage should not be registered before Start returns success")
		}
	}
	return tt.startErr
}
func (tt *loaderTestTask) Close() error { return nil }

func hasRegisteredStore(r *URLStateRegistry, taskID string) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.stores {
		if entry.taskID == taskID && entry.store != nil {
			return true
		}
	}
	return false
}

func TestTaskLoader_RangeAddressDoesNotReuseConfigPointer(t *testing.T) {
	typ := "test_task_loader_cfg_ptr"
	cfgPtrs := make([]uintptr, 0, 2)

	task.Register(typ, func(cfg *config.Task, opts task.Options) (core.Task, error) {
		cfgPtrs = append(cfgPtrs, uintptr(unsafe.Pointer(cfg)))
		return &loaderTestTask{id: cfg.ID, typ: typ}, nil
	})

	mgr := NewManager(&config.Config{
		Tasks: []config.Task{
			{ID: "t1", Type: typ},
			{ID: "t2", Type: typ},
		},
	})
	mgr.loadTasks()

	if len(cfgPtrs) != 2 {
		t.Fatalf("expected 2 task creations, got %d", len(cfgPtrs))
	}
	if cfgPtrs[0] == cfgPtrs[1] {
		t.Fatalf("expected distinct cfg pointers per task creation")
	}
}

func TestTaskLoader_RegistersStorageAfterStartSuccess(t *testing.T) {
	typ := "test_task_loader_start_order"
	cfg := &config.Config{
		Tasks: []config.Task{
			{
				ID:   "t1",
				Type: typ,
			},
		},
	}
	mgr := NewManager(cfg)

	store, err := storage.NewStorage("memory", nil)
	if err != nil {
		t.Fatalf("new storage err: %v", err)
	}

	task.Register(typ, func(cfg *config.Task, opts task.Options) (core.Task, error) {
		return &loaderTestTask{
			t:                t,
			mgr:              mgr,
			id:               cfg.ID,
			typ:              typ,
			store:            store,
			checkBeforeStore: true,
		}, nil
	})

	mgr.loadTasks()
	if _, ok := mgr.tasks.Load("t1"); !ok {
		t.Fatalf("expected task stored after Start success")
	}
	if !hasRegisteredStore(mgr.urlRegistry, "t1") {
		t.Fatalf("expected storage registered after Start success")
	}
}

func TestTaskLoader_StartFailLeavesNoResidue(t *testing.T) {
	typ := "test_task_loader_start_fail"
	cfg := &config.Config{
		Tasks: []config.Task{
			{
				ID:   "t1",
				Type: typ,
			},
		},
	}
	mgr := NewManager(cfg)

	store, err := storage.NewStorage("memory", nil)
	if err != nil {
		t.Fatalf("new storage err: %v", err)
	}

	task.Register(typ, func(cfg *config.Task, opts task.Options) (core.Task, error) {
		return &loaderTestTask{
			t:        t,
			mgr:      mgr,
			id:       cfg.ID,
			typ:      typ,
			store:    store,
			startErr: errors.New("start failed"),
		}, nil
	})

	mgr.loadTasks()
	if _, ok := mgr.tasks.Load("t1"); ok {
		t.Fatalf("expected task not stored on Start failure")
	}
	if hasRegisteredStore(mgr.urlRegistry, "t1") {
		t.Fatalf("expected storage not registered on Start failure")
	}
}
