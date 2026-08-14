// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
)

// versionedTask 实现 core.ObjectVersioner，用内存存储。
type versionedTask struct {
	*mockTask
	store      core.Storage
	latest     int
	beginCount int
	stepCalls  []int
	upgradeFn  func(obj *model.DownloadObject, toVersion int) (bool, error)
}

func (v *versionedTask) Storage() core.Storage { return v.store }
func (v *versionedTask) LatestVersion() int    { return v.latest }
func (v *versionedTask) BeginUpgrade()         { v.beginCount++ }
func (v *versionedTask) UpgradeStep(obj *model.DownloadObject, toVersion int) (bool, error) {
	v.stepCalls = append(v.stepCalls, toVersion)
	if v.upgradeFn != nil {
		return v.upgradeFn(obj, toVersion)
	}
	return false, nil
}

func newVersionTask(t *testing.T, latest int, upgradeFn func(*model.DownloadObject, int) (bool, error)) (*versionedTask, core.Storage) {
	t.Helper()
	store, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatalf("NewMemoryStorage: %v", err)
	}
	return &versionedTask{
		mockTask:  &mockTask{id: "t1", typ: "v1"},
		store:     store,
		latest:    latest,
		upgradeFn: upgradeFn,
	}, store
}

func TestRunVersionUpgrade_StepsAndPersists(t *testing.T) {
	cfg := &config.Config{Tasks: []config.Task{{ID: "t1", Type: "v1"}}}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)
	vt, store := newVersionTask(t, 2, func(obj *model.DownloadObject, to int) (bool, error) {
		if to == 1 {
			obj.Extra["migrated1"] = true
		}
		if to == 2 {
			obj.Extra["migrated2"] = true
		}
		return true, nil
	})

	old := &model.DownloadObject{URL: "old", TaskID: "t1", Version: 0, Extra: map[string]any{}}
	cur := &model.DownloadObject{URL: "cur", TaskID: "t1", Version: 2, Extra: map[string]any{}}
	if err := store.Update(old); err != nil {
		t.Fatalf("seed old: %v", err)
	}
	if err := store.Update(cur); err != nil {
		t.Fatalf("seed cur: %v", err)
	}
	m.tasks.Store("t1", vt)

	NewStandardizationService(m).runVersionUpgrade(t.Context())

	if vt.beginCount != 1 {
		t.Errorf("BeginUpgrade count = %d, want 1", vt.beginCount)
	}
	// 只扫描 version<2 的对象（old），cur 已到最新不参与。
	if len(vt.stepCalls) != 2 || vt.stepCalls[0] != 1 || vt.stepCalls[1] != 2 {
		t.Errorf("stepCalls = %v, want [1 2]", vt.stepCalls)
	}
	got, err := store.Get("old")
	if err != nil || got == nil {
		t.Fatalf("Get old: %v", err)
	}
	if got.GetVersion() != 2 {
		t.Errorf("old version = %d, want 2", got.GetVersion())
	}
	if got.Extra["migrated1"] != true || got.Extra["migrated2"] != true {
		t.Errorf("old extra = %v, want migrated1+migrated2", got.Extra)
	}
}

func TestRunVersionUpgrade_PersistsVersionEvenWithoutContentChange(t *testing.T) {
	cfg := &config.Config{Tasks: []config.Task{{ID: "t1", Type: "v1"}}}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)
	// 幂等无内容改动 → version 仍应收敛到最新。
	vt, store := newVersionTask(t, 3, func(*model.DownloadObject, int) (bool, error) { return false, nil })
	obj := &model.DownloadObject{URL: "u", TaskID: "t1", Version: 0, Extra: map[string]any{}}
	if err := store.Update(obj); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m.tasks.Store("t1", vt)

	NewStandardizationService(m).runVersionUpgrade(t.Context())

	got, err := store.Get("u")
	if err != nil || got == nil {
		t.Fatalf("Get: %v", err)
	}
	if got.GetVersion() != 3 {
		t.Errorf("version = %d, want 3 (persisted even with no content change)", got.GetVersion())
	}
}

func TestRunVersionUpgrade_ContextCancel(t *testing.T) {
	cfg := &config.Config{Tasks: []config.Task{{ID: "t1", Type: "v1"}}}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)
	vt, store := newVersionTask(t, 1, func(*model.DownloadObject, int) (bool, error) { return true, nil })
	obj := &model.DownloadObject{URL: "u", TaskID: "t1", Version: 0, Extra: map[string]any{}}
	if err := store.Update(obj); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m.tasks.Store("t1", vt)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // 已取消
	NewStandardizationService(m).runVersionUpgrade(ctx)

	if len(vt.stepCalls) != 0 {
		t.Errorf("stepCalls = %v, want none after ctx cancel", vt.stepCalls)
	}
}

func TestRunVersionUpgrade_SkipsNonVersionerAndNilStorage(t *testing.T) {
	cfg := &config.Config{Tasks: []config.Task{{ID: "t1", Type: "plain"}}}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)
	m.tasks.Store("t1", &mockTask{id: "t1", typ: "plain"}) // 非 ObjectVersioner
	// 不应 panic
	NewStandardizationService(m).runVersionUpgrade(t.Context())
}
