// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// fakeGroupRepStorage 实现 core.ContentGroupRepresentatives，用于验证 GetTaskContentGroups 快路径。
type fakeGroupRepStorage struct {
	objs  []*model.DownloadObject
	total int64
}

func (f *fakeGroupRepStorage) Get(string) (*model.DownloadObject, error) { return nil, nil }
func (f *fakeGroupRepStorage) Update(*model.DownloadObject) error        { return nil }
func (f *fakeGroupRepStorage) Delete(string) error                       { return nil }
func (f *fakeGroupRepStorage) Search(*core.StorageQuery) ([]*model.DownloadObject, error) {
	return nil, nil
}
func (f *fakeGroupRepStorage) Count(*core.StorageQuery) (int64, error)  { return 0, nil }
func (f *fakeGroupRepStorage) Exists([]string) (map[string]bool, error) { return nil, nil }
func (f *fakeGroupRepStorage) ContentGroupRepresentatives(taskID, search, status string, page, limit int64) ([]*model.DownloadObject, int64, error) {
	return f.objs, f.total, nil
}

// groupRepTask 用 Storage() 返回指定存储的任务。
type groupRepTask struct {
	*mockTask
	store core.Storage
}

func (g *groupRepTask) Storage() core.Storage { return g.store }

func TestGetTaskContentGroups_FastPath(t *testing.T) {
	cfg := &config.Config{Tasks: []config.Task{{ID: "t1", Type: "mxs"}}}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)

	fake := &fakeGroupRepStorage{
		total: 2,
		objs: []*model.DownloadObject{
			{URL: "rep-1095", Metadata: map[string]string{"content_group": "1095", "date": "0000000020"}, Extra: map[string]any{}},
			{URL: "rep-1094", Metadata: map[string]string{"content_group": "1094", "date": "0000000003"}, Extra: map[string]any{}},
		},
	}
	m.tasks.Store("t1", &groupRepTask{mockTask: &mockTask{id: "t1", typ: "mxs"}, store: fake})

	res, err := m.GetTaskContentGroups("t1", 1, 50)
	if err != nil {
		t.Fatalf("GetTaskContentGroups: %v", err)
	}
	objs, ok := res["objects"].([]*model.DownloadObject)
	if !ok {
		t.Fatalf("objects type = %T", res["objects"])
	}
	if len(objs) != 2 {
		t.Fatalf("len(objs) = %d, want 2", len(objs))
	}
	if res["total"].(int64) != 2 {
		t.Errorf("total = %v, want 2", res["total"])
	}
	if objs[0].URL != "rep-1095" || objs[0].GetMetaContentGroup() != "1095" {
		t.Errorf("objs[0] = %s %s", objs[0].URL, objs[0].GetMetaContentGroup())
	}
	// 快路径也应补齐 task_type（前端插件分发依赖）
	if objs[0].GetMetaTaskType() != "mxs" {
		t.Errorf("objs[0] task_type = %q, want mxs", objs[0].GetMetaTaskType())
	}
	// group_size 由存储层 SetGroupSize 写入，应原样透传
	objs[1].Extra["group_size"] = 3
	if objs[1].GetGroupSize() != 3 {
		t.Errorf("objs[1] group_size = %d, want 3", objs[1].GetGroupSize())
	}
}

func TestGetTaskContentGroups_FastPath_NormalizesLimitAll(t *testing.T) {
	cfg := &config.Config{Tasks: []config.Task{{ID: "t1", Type: "mxs"}}}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)

	fake := &fakeGroupRepStorage{
		total: 7,
		objs: []*model.DownloadObject{
			{URL: "rep-1", Metadata: map[string]string{"content_group": "g1", "date": "2"}, Extra: map[string]any{}},
		},
	}
	m.tasks.Store("t1", &groupRepTask{mockTask: &mockTask{id: "t1", typ: "mxs"}, store: fake})

	// limit<=0 → page 归 1、limit 归 total（与 fallback 响应形状一致）
	res, err := m.GetTaskContentGroups("t1", 3, -1)
	if err != nil {
		t.Fatalf("GetTaskContentGroups: %v", err)
	}
	if res["page"].(int64) != 1 {
		t.Errorf("page = %v, want 1", res["page"])
	}
	if res["limit"].(int64) != 7 {
		t.Errorf("limit = %v, want 7", res["limit"])
	}
}

func TestAggregateByContent_FastPathForNonVariantTask(t *testing.T) {
	cfg := &config.Config{Tasks: []config.Task{{ID: "t1", Type: "mxs"}}}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)

	// mxs 非变体任务 → 走存储层 ContentGroupRepresentatives 快路径，不经过内存收集。
	fake := &fakeGroupRepStorage{
		total: 2,
		objs: []*model.DownloadObject{
			{TaskID: "t1", URL: "rep-1095", Metadata: map[string]string{"content_group": "1095", "date": "0000000020"}, Extra: map[string]any{}},
			{TaskID: "t1", URL: "rep-1094", Metadata: map[string]string{"content_group": "1094", "date": "0000000003"}, Extra: map[string]any{}},
		},
	}
	m.tasks.Store("t1", &groupRepTask{mockTask: &mockTask{id: "t1", typ: "mxs"}, store: fake})

	res, err := m.AggregateByContent(1, 50, "", "date_desc", "all", nil)
	if err != nil {
		t.Fatalf("AggregateByContent: %v", err)
	}
	objs, ok := res["objects"].([]*model.DownloadObject)
	if !ok {
		t.Fatalf("objects type = %T", res["objects"])
	}
	if len(objs) != 2 {
		t.Fatalf("len(objs) = %d, want 2", len(objs))
	}
	if res["total"].(int64) != 2 {
		t.Errorf("total = %v, want 2", res["total"])
	}
	// 按 date 降序：rep-1095(0000000020) 在前
	if objs[0].URL != "rep-1095" || objs[1].URL != "rep-1094" {
		t.Errorf("order = %s, %s", objs[0].URL, objs[1].URL)
	}
	// 快路径 rep 的 group_size 应由存储层透传
	objs[1].Extra["group_size"] = 3
	if objs[1].GetGroupSize() != 3 {
		t.Errorf("objs[1] group_size = %d, want 3", objs[1].GetGroupSize())
	}
}

func TestGetTaskContentGroups_FallbackPath(t *testing.T) {
	cfg := &config.Config{Tasks: []config.Task{{ID: "t1", Type: "mxs"}}}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)

	// 使用纯 mockTask（Storage() 返回 nil，走 GetAllObjects fallback）。
	o1 := &model.DownloadObject{
		TaskID:   "t1",
		URL:      "u1",
		Metadata: map[string]string{"content_group": "1094", "date": "0000000001"},
		Extra:    map[string]any{},
	}
	o2 := &model.DownloadObject{
		TaskID:   "t1",
		URL:      "u2",
		Metadata: map[string]string{"content_group": "1094", "date": "0000000003"},
		Extra:    map[string]any{},
	}
	o3 := &model.DownloadObject{
		TaskID:   "t1",
		URL:      "u3",
		Metadata: map[string]string{"content_group": "1095", "date": "0000000020"},
		Extra:    map[string]any{},
	}
	m.tasks.Store("t1", &mockTask{id: "t1", typ: "mxs", objs: []*model.DownloadObject{o1, o2, o3}})

	res, err := m.GetTaskContentGroups("t1", 1, 50)
	if err != nil {
		t.Fatalf("GetTaskContentGroups: %v", err)
	}
	objs, ok := res["objects"].([]*model.DownloadObject)
	if !ok {
		t.Fatalf("objects type = %T", res["objects"])
	}
	// 两个组，按代表 date 降序：1095(0000000020) 在 1094(0000000003) 之前。
	if len(objs) != 2 {
		t.Fatalf("len(objs) = %d, want 2", len(objs))
	}
	if objs[0].GetMetaContentGroup() != "1095" || objs[1].GetMetaContentGroup() != "1094" {
		t.Errorf("groups order = %s, %s", objs[0].GetMetaContentGroup(), objs[1].GetMetaContentGroup())
	}
	if objs[1].GetMetaDate() != "0000000003" {
		t.Errorf("objs[1] date = %q, want newest 0000000003", objs[1].GetMetaDate())
	}
	if objs[1].GetGroupSize() != 2 {
		t.Errorf("objs[1] group_size = %d, want 2", objs[1].GetGroupSize())
	}
}
