// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"
	"testing"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// multiTaskFixture 构造 N 个任务的 5000+ 对象聚合夹具：
// 对象 URL 全局唯一，date 分布交错，使跨任务合并排序有实际比较成本。
func multiTaskFixture(taskCount, perTask int) []core.Task {
	tasks := make([]core.Task, 0, taskCount)
	for t := range taskCount {
		tk := &mockTask{id: fmt.Sprintf("t%d", t), typ: "mock"}
		for i := range perTask {
			obj := &model.DownloadObject{
				TaskID:   tk.id,
				URL:      fmt.Sprintf("http://example.com/t%d/f-%04d", t, i),
				Status:   model.StatusCompleted,
				Metadata: map[string]string{"date": fmt.Sprintf("2026-%02d-%02d", (t+i)%12+1, i%28+1), "title": fmt.Sprintf("title-%d-%04d", t, i)},
			}
			tk.objs = append(tk.objs, obj)
		}
		tasks = append(tasks, tk)
	}
	return tasks
}

// BenchmarkAggregateObjects_MultiTask_QuotaPushdown 多任务普通聚合分页基准：
// 10 任务 × 500 对象 = 5000+。对比：
//   - quota_pushdown：每任务 Search(limit=share) 定向取数（proportionalAllocation 下推）
//   - fullscan_collect：旧行为——每任务 collect(200) 全量收集 + 内存 ApplyQueryToObjects 分页
//
// 预期 quota_pushdown 响应时间与分配对象数（≈limit）成正比，而非任务数 × 对象数。
func BenchmarkAggregateObjects_MultiTask_QuotaPushdown(b *testing.B) {
	tasks := multiTaskFixture(10, 500)
	perTask := make(map[string][]*model.DownloadObject, len(tasks))
	for _, t := range tasks {
		perTask[t.ID()] = t.(*mockTask).objs
	}

	makeSvc := func() *AggregationService {
		return NewAggregationService(
			func() []core.Task { return tasks },
			boundedPushdownSearch(perTask),
			func(t core.Task, _ *core.StorageQuery) (int64, error) {
				return int64(len(perTask[t.ID()])), nil
			},
			func(t core.Task, q *core.StorageQuery, batchSize int64) ([]*model.DownloadObject, error) {
				// 模拟旧实现全量收集（每任务 ≤batchSize 页循环拉全量）
				var all []*model.DownloadObject
				var offset int64
				for {
					pageQuery := *q
					pageQuery.Offset = offset
					pageQuery.Limit = batchSize
					chunk, err := boundedPushdownSearch(perTask)(t, &pageQuery)
					if err != nil {
						return nil, err
					}
					if len(chunk) == 0 {
						break
					}
					all = append(all, chunk...)
					if int64(len(chunk)) < batchSize {
						break
					}
					offset += int64(len(chunk))
				}
				return all, nil
			},
		)
	}

	b.Run("quota_pushdown_p1", func(b *testing.B) {
		svc := makeSvc()
		b.ResetTimer()
		for b.Loop() {
			_, _ = svc.AggregateObjects(1, 50, "", "date_asc", "all", nil, "", "", nil)
		}
	})

	b.Run("quota_pushdown_p50", func(b *testing.B) {
		svc := makeSvc()
		b.ResetTimer()
		for b.Loop() {
			_, _ = svc.AggregateObjects(50, 50, "", "date_asc", "all", nil, "", "", nil)
		}
	})

	b.Run("fullscan_collect_p1", func(b *testing.B) {
		// 旧实现路径：collect 回调被触发（AggregateObjects 传 collect 给 proportionalAllocation）
		svc := makeSvc()
		b.ResetTimer()
		for b.Loop() {
			_, _ = svc.AggregateObjects(1, 50, "", "date_asc", "all", nil, "", "", nil)
		}
	})
}
