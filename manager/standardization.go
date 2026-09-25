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
//  1. ID 回填：对实现 Standardizer 的任务类型，对 MissingID=true 的对象执行标准化。
//  2. 版本升级：对实现 core.ObjectVersioner 的任务，扫描 version < LatestVersion 的对象，
//     按预设逻辑逐级升级到最新结构。
//
// ctx 用于取消控制，每次对象处理前检查 ctx.Done()。
func (s *StandardizationService) Run(ctx context.Context) {
	s.runIDStandardization(ctx)
	s.runVersionUpgrade(ctx)
}

// runIDStandardization 存量 ID 回填。
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

// runVersionUpgrade 扫描实现 core.ObjectVersioner 的任务类型中 version < LatestVersion 的对象，
// 逐级调用 UpgradeStep 升级到最新结构。BeginUpgrade 在扫描开始前清理跨对象缓存；
// 每个对象至少跑过一个升级步骤（即使无内容改动）也会持久化，确保 version 在存储中收敛。
func (s *StandardizationService) runVersionUpgrade(ctx context.Context) {
	for _, taskType := range s.mgr.UniqueTaskTypes() {
		task := s.mgr.FirstTaskOfType(taskType)
		if task == nil {
			continue
		}
		ov, ok := task.(core.ObjectVersioner)
		if !ok {
			continue
		}
		latest := int64(ov.LatestVersion())
		if latest <= 0 {
			continue
		}
		st := task.Storage()
		if st == nil {
			continue
		}
		ov.BeginUpgrade()

		objs, err := s.mgr.collectTaskObjects(task, &core.StorageQuery{
			Filter: core.StorageFilter{VersionLT: latest},
			Limit:  core.NoLimit,
		}, core.NoLimit)
		if err != nil {
			slog.Error("VersionUpgrade: search failed", logutil.LogKeyTaskID, task.ID(),
				"task_type", taskType, logutil.LogKeyError, err)
			continue
		}
		if len(objs) == 0 {
			continue
		}

		count := 0
		for _, obj := range objs {
			select {
			case <-ctx.Done():
				slog.Warn("VersionUpgrade cancelled", "task_type", taskType, "processed", count)
				return
			default:
			}
			cur := obj.GetVersion()
			for cur < latest {
				modified, err := ov.UpgradeStep(obj, int(cur+1))
				if err != nil {
					slog.Error("VersionUpgrade: step failed", logutil.LogKeyTaskID, task.ID(),
						logutil.LogKeyURL, obj.URL, "to_version", cur+1, logutil.LogKeyError, err)
					break
				}
				obj.SetVersion(cur + 1)
				cur++
				_ = modified
			}
			if err := st.Update(obj); err != nil {
				slog.Error("VersionUpgrade: update failed", logutil.LogKeyTaskID, task.ID(),
					logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
				continue
			}
			count++
		}

		slog.Info("VersionUpgrade completed", "task_type", taskType, "processed", count)
	}
}
