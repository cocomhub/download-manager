// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"log/slog"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

// StandardizationService 扫描存量旧数据，执行标准化操作（如 ID 提取）。
// 每种任务类型只处理一次，与任务实例数量无关。
type StandardizationService struct {
	mgr *Manager
}

func NewStandardizationService(mgr *Manager) *StandardizationService {
	return &StandardizationService{mgr: mgr}
}

// Run 执行一次标准化扫描，分两个阶段：
//  1. 存量 ID 回填：遍历所有任务类型，对实现 Standardizer 的任务，
//     对 MissingID=true 的对象执行标准化。
//  2. 媒体固定字段回填：对实现 core.SmallObjectProvider 的任务，
//     扫描全部对象，把 cover/thumb/preview 的原始 URL 与本地路径
//     写入固定字段 {rel}_url / {rel}_path（初始化时自动更新，统一各文档行为）。
//
// ctx 用于取消控制，每个对象处理前检查 ctx.Done()。
func (s *StandardizationService) Run(ctx context.Context) {
	s.runIDStandardization(ctx)
	s.runMediaBackfill(ctx)
}

// runIDStandardization 对 MissingID 对象执行 Standardizer 回填。
func (s *StandardizationService) runIDStandardization(ctx context.Context) {
	for _, taskType := range s.mgr.UniqueTaskTypes() {
		task := s.mgr.FirstTaskOfType(taskType)
		if task == nil {
			slog.Debug("Standardization: no task found for type", "task_type", taskType)
			continue
		}
		std, ok := task.(core.Standardizer)
		if !ok {
			slog.Debug("Standardization: task type does not implement Standardizer",
				logutil.LogKeyTaskID, task.ID(), "task_type", taskType)
			continue
		}

		st := task.Storage()
		if st == nil {
			continue
		}

		missingID := true
		objects, err := st.Search(&core.StorageQuery{
			Filter: core.StorageFilter{
				MissingID: &missingID,
			},
			Limit: core.NoLimit, // 不限量
		})
		if err != nil {
			slog.Error("Standardization: search failed", logutil.LogKeyTaskID, task.ID(),
				"task_type", taskType, logutil.LogKeyError, err)
			continue
		}

		if len(objects) == 0 {
			continue
		}

		count := 0
		for _, obj := range objects {
			select {
			case <-ctx.Done():
				slog.Warn("Standardization cancelled", "task_type", taskType, "processed", count)
				return
			default:
			}
			if modified, err := std.Standardize(obj); err != nil {
				slog.Error("Standardization: failed", logutil.LogKeyTaskID, task.ID(),
					logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
			} else if modified {
				if err := st.Update(obj); err != nil {
					slog.Error("Standardization: update failed", logutil.LogKeyTaskID, task.ID(),
						logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
				} else {
					count++
				}
			}
		}

		slog.Info("Standardization completed", "task_type", taskType, "processed", count)
	}
}

// runMediaBackfill 对实现 core.SmallObjectProvider 的任务类型，扫描其全部对象，
// 把 cover/thumb/preview 的原始 URL 与本地保存路径写入固定字段 {rel}_url / {rel}_path。
// 在启动期执行（无并发下载），仅在有改动时持久化，幂等。
func (s *StandardizationService) runMediaBackfill(ctx context.Context) {
	for _, taskType := range s.mgr.UniqueTaskTypes() {
		task := s.mgr.FirstTaskOfType(taskType)
		if task == nil {
			continue
		}
		soc, ok := task.(core.SmallObjectProvider)
		if !ok {
			continue
		}
		st := task.Storage()
		if st == nil {
			continue
		}

		objects, err := st.Search(&core.StorageQuery{Filter: core.StorageFilter{}, Limit: core.NoLimit})
		if err != nil {
			slog.Error("Media backfill: search failed", logutil.LogKeyTaskID, task.ID(),
				"task_type", taskType, logutil.LogKeyError, err)
			continue
		}

		count := 0
		for _, obj := range objects {
			select {
			case <-ctx.Done():
				slog.Warn("Media backfill cancelled", "task_type", taskType, "processed", count)
				return
			default:
			}
			modified := false
			for _, info := range soc.SmallObjects(obj) {
				if info.URL == "" {
					continue
				}
				u, p := obj.GetMedia(info.Rel)
				if u != info.URL || p != info.SavePath {
					obj.SetMedia(info.Rel, info.URL, info.SavePath)
					modified = true
				}
			}
			if modified {
				if err := st.Update(obj); err != nil {
					slog.Error("Media backfill: update failed", logutil.LogKeyTaskID, task.ID(),
						logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
				} else {
					count++
				}
			}
		}
		slog.Info("Media backfill completed", "task_type", taskType, "processed", count)
	}
}
