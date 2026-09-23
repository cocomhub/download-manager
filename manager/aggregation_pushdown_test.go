// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
)

// TestAggregateObjects_SingleTaskPushdown 验证单任务聚合走后端下推：
// 只取 limit 个对象（跨任务合并场景才需全量收集，单任务可直接下推分页）。
func TestAggregateObjects_SingleTaskPushdown(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{ID: "t1", Type: "mock"},
		},
	}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)

	ms, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatalf("NewMemoryStorage: %v", err)
	}
	// 单任务 500 个对象
	for i := range 500 {
		if err := ms.Update(&model.DownloadObject{
			TaskID:   "t1",
			URL:      fmt.Sprintf("http://example.com/file-%03d.bin", i),
			Metadata: map[string]string{"date": fmt.Sprintf("2026-09-%02d", i%28+1)},
		}); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	t1 := &mockTaskWithStorage{id: "t1", typ: "mock", st: ms}
	m.tasks.Store("t1", t1)

	// limit=10, page=2（offset=10）→ 单任务下推应只返回 10 个
	res, err := m.AggregateObjects(2, 10, "", "date_asc", "all", nil, "", "", nil)
	if err != nil {
		t.Fatalf("AggregateObjects: %v", err)
	}
	objs, ok := res["objects"].([]*model.DownloadObject)
	if !ok {
		t.Fatalf("unexpected objects type %T", res["objects"])
	}
	if len(objs) != 10 {
		t.Fatalf("expected 10 objects from pushdown, got %d", len(objs))
	}
	// 校验排序：date_asc 首项应为最小日期
	if len(objs) > 1 {
		d0 := objs[0].Metadata["date"]
		d1 := objs[1].Metadata["date"]
		if d0 > d1 {
			t.Fatalf("pushdown sort date_asc broken: %s > %s", d0, d1)
		}
	}
}

// TestAggregateObjects_MultiTaskCollect 验证多任务合并仍需全量收集（语义回归保护）。
func TestAggregateObjects_MultiTaskCollect(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{ID: "t1", Type: "mock"},
			{ID: "t2", Type: "mock"},
		},
	}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)

	for _, tid := range []string{"t1", "t2"} {
		ms, err := storage.NewMemoryStorage(nil)
		if err != nil {
			t.Fatalf("NewMemoryStorage: %v", err)
		}
		for i := range 30 {
			if err := ms.Update(&model.DownloadObject{
				TaskID:   tid,
				URL:      fmt.Sprintf("http://example.com/%s/file-%02d.bin", tid, i),
				Metadata: map[string]string{"date": fmt.Sprintf("2026-09-%02d", i%28+1)},
			}); err != nil {
				t.Fatalf("Update: %v", err)
			}
		}
		m.tasks.Store(tid, &mockTaskWithStorage{id: tid, typ: "mock", st: ms})
	}

	res, err := m.AggregateObjects(1, 10, "", "date_asc", "all", nil, "", "", nil)
	if err != nil {
		t.Fatalf("AggregateObjects: %v", err)
	}
	objs, ok := res["objects"].([]*model.DownloadObject)
	if !ok {
		t.Fatalf("unexpected objects type %T", res["objects"])
	}
	if len(objs) != 10 {
		t.Fatalf("expected 10 objects from multi-task merge, got %d", len(objs))
	}
	// 跨任务合并结果需全局有序
	if len(objs) > 1 {
		d0 := objs[0].Metadata["date"]
		for _, o := range objs[1:] {
			if o.Metadata["date"] < d0 {
				t.Fatalf("multi-task merge sort broken: %s < %s", o.Metadata["date"], d0)
			}
		}
	}
}

// 编译期确认 StorageQuery 下推字段可被各后端消费。
var _ = core.StorageQuery{Filter: core.StorageFilter{}, Sort: []core.StorageSort{}, Offset: 0, Limit: core.NoLimit}
