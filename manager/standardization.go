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

// Run 执行一次标准化扫描。
// 遍历所有任务类型，取一个 Task 实例，检查是否实现 Standardizer，
// 对 MissingID=true 的对象执行标准化。
// ctx 用于取消控制，每次对象处理前检查 ctx.Done()。
func (s *StandardizationService) Run(ctx context.Context) {
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
			Limit: -1, // -1 = 不限量（避免 MongoDB normalizeMongoQuery 将 0 钳位到 200）
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
