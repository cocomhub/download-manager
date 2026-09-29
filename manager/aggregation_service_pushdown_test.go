// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// stubCount 构造一个 mockTask（objs 升序 date）并返回其对象列表。
func stubCount(id string, count int) (core.Task, []*model.DownloadObject) {
	t := &mockTask{id: id, typ: "mock"}
	var objs []*model.DownloadObject
	for i := range count {
		obj := &model.DownloadObject{
			TaskID:   id,
			URL:      fmt.Sprintf("%s-%03d", id, i),
			Metadata: map[string]string{"date": fmt.Sprintf("2026-09-%02d", i%28+1)},
		}
		objs = append(objs, obj)
		t.objs = append(t.objs, obj)
	}
	return t, objs
}

// boundedPushdownSearch 模拟支持 limit/offset/sort 的后端 Search：
// 返回该任务排序后 [Offset, Offset+Limit) 子集（与 mongo/file/memory 后端一致）。
func boundedPushdownSearch(perTask map[string][]*model.DownloadObject) func(core.Task, *core.StorageQuery) ([]*model.DownloadObject, error) {
	return func(t core.Task, q *core.StorageQuery) ([]*model.DownloadObject, error) {
		list := perTask[t.ID()]
		sorted := append([]*model.DownloadObject(nil), list...)
		sort.SliceStable(sorted, func(i, j int) bool {
			return sorted[i].Metadata["date"] < sorted[j].Metadata["date"]
		})
		offset := max(q.Offset, 0)
		if offset >= int64(len(sorted)) {
			return []*model.DownloadObject{}, nil
		}
		end := offset + q.Limit
		if q.Limit <= 0 || end > int64(len(sorted)) {
			end = int64(len(sorted))
		}
		return sorted[offset:end], nil
	}
}

// recordSearch 包装 boundedPushdownSearch 并记录每个任务收到的查询（Limit/Offset）。
func recordSearch(perTask map[string][]*model.DownloadObject, calls map[string][]core.StorageQuery, mu *sync.Mutex) func(core.Task, *core.StorageQuery) ([]*model.DownloadObject, error) {
	underlying := boundedPushdownSearch(perTask)
	return func(t core.Task, q *core.StorageQuery) ([]*model.DownloadObject, error) {
		mu.Lock()
		calls[t.ID()] = append(calls[t.ID()], *q)
		mu.Unlock()
		return underlying(t, q)
	}
}

// TestAggregateObjects_MultiTask_QuotaPushdown 验证多任务普通聚合分页下推：
// proportionalAllocation 按各任务计数比例分配页内配额，并对每个任务用
// Search(Limit=share, Offset=(page-1)*share) 定向取数（不得再拉 limit*3 超集）。
//
// 数据：t1=90 条、t2=40 条，total=130，limit=5。
// share(t1) = max(1, 5*90/130) = 3；share(t2, 末位吃剩余) = 5-3 = 2。
// page=2 期望查询：t1 → offset=3, limit=3；t2 → offset=2, limit=2。
func TestAggregateObjects_MultiTask_QuotaPushdown(t *testing.T) {
	t1, objs1 := stubCount("t1", 90)
	t2, objs2 := stubCount("t2", 40)
	perTask := map[string][]*model.DownloadObject{
		"t1": objs1,
		"t2": objs2,
	}

	var mu sync.Mutex
	calls := make(map[string][]core.StorageQuery)
	svc := NewAggregationService(
		func() []core.Task { return []core.Task{t1, t2} },
		recordSearch(perTask, calls, &mu),
		func(t core.Task, _ *core.StorageQuery) (int64, error) {
			return int64(len(perTask[t.ID()])), nil
		},
		nil, // collect 不应被调用：多任务限流查询应走每任务 search 配额
	)

	page, limit := int64(2), int64(5)
	res, err := svc.AggregateObjects(page, limit, "", "date_asc", "all", nil, "", "", nil)
	if err != nil {
		t.Fatalf("AggregateObjects: %v", err)
	}
	objs, ok := res["objects"].([]*model.DownloadObject)
	if !ok {
		t.Fatalf("unexpected objects type %T", res["objects"])
	}
	if len(objs) != int(limit) {
		t.Fatalf("expected exactly %d objects on page %d, got %d", limit, page, len(objs))
	}
	// 合并结果需全局有序（date_asc）
	for i := 1; i < len(objs); i++ {
		if objs[i].Metadata["date"] < objs[i-1].Metadata["date"] {
			t.Fatalf("page %d result not globally sorted: %s < %s", page, objs[i].Metadata["date"], objs[i-1].Metadata["date"])
		}
	}

	mu.Lock()
	defer mu.Unlock()
	t1Calls, t2Calls := calls["t1"], calls["t2"]
	if len(t1Calls) == 0 || len(t2Calls) == 0 {
		t.Fatalf("expected search calls for both tasks, got t1=%d t2=%d", len(t1Calls), len(t2Calls))
	}
	for _, q := range append(append([]core.StorageQuery{}, t1Calls...), t2Calls...) {
		if q.Limit > limit {
			t.Errorf("search Limit %d exceeds page limit %d (superset fetch)", q.Limit, limit)
		}
	}
	// 页内偏移推进：每任务 offset 应为 (page-1)*share
	var t1OK, t2OK bool
	for _, q := range t1Calls {
		if q.Limit == 3 && q.Offset == 3 {
			t1OK = true
		}
	}
	for _, q := range t2Calls {
		if q.Limit == 2 && q.Offset == 2 {
			t2OK = true
		}
	}
	if !t1OK {
		t.Errorf("t1 missing page-aware quota query (limit=3, offset=3), got %+v", t1Calls)
	}
	if !t2OK {
		t.Errorf("t2 missing page-aware quota query (limit=2, offset=2), got %+v", t2Calls)
	}
}

// TestAggregateObjects_MultiTask_QuotaOffsetBeyond 验证页偏移越界（无更多数据）
// 时返回空页而不报错（配额下推的边界：每任务 offset 可能超过其自身对象数）。
func TestAggregateObjects_MultiTask_QuotaOffsetBeyond(t *testing.T) {
	t1, objs1 := stubCount("t1", 10)
	t2, objs2 := stubCount("t2", 10)
	perTask := map[string][]*model.DownloadObject{"t1": objs1, "t2": objs2}

	svc := NewAggregationService(
		func() []core.Task { return []core.Task{t1, t2} },
		boundedPushdownSearch(perTask),
		func(t core.Task, _ *core.StorageQuery) (int64, error) {
			return int64(len(perTask[t.ID()])), nil
		},
		nil,
	)

	// total=20, limit=5, page=5 → offset=20 ≥ total → 空页
	res, err := svc.AggregateObjects(5, 5, "", "date_asc", "all", nil, "", "", nil)
	if err != nil {
		t.Fatalf("AggregateObjects: %v", err)
	}
	objs, ok := res["objects"].([]*model.DownloadObject)
	if !ok {
		t.Fatalf("unexpected objects type %T", res["objects"])
	}
	if len(objs) != 0 {
		t.Fatalf("expected empty page beyond total, got %d objects", len(objs))
	}
}

// TestAggregateObjects_SingleTask_NoCollect 回归保护：单任务限流查询
// 必须走 storage.Search 下推，不得回退 collect 全量收集。
func TestAggregateObjects_SingleTask_NoCollect(t *testing.T) {
	t1, objs1 := stubCount("t1", 100)
	perTask := map[string][]*model.DownloadObject{"t1": objs1}
	collectUsed := false
	svc := NewAggregationService(
		func() []core.Task { return []core.Task{t1} },
		boundedPushdownSearch(perTask),
		func(_ core.Task, _ *core.StorageQuery) (int64, error) { return 100, nil },
		func(_ core.Task, _ *core.StorageQuery, _ int64) ([]*model.DownloadObject, error) {
			collectUsed = true
			return nil, nil
		},
	)

	if _, err := svc.AggregateObjects(1, 10, "", "date_asc", "all", nil, "", "", nil); err != nil {
		t.Fatalf("AggregateObjects: %v", err)
	}
	if collectUsed {
		t.Fatal("single-task aggregate with limit should use storage.Search pushdown, not collect")
	}
}
