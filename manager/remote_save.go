// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"fmt"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/remotesave"
)

// TriggerRemoteSave 手动触发任务的全部 completed 对象同步（force=true 忽略 manifest）。
func (m *Manager) TriggerRemoteSave(taskID string, force bool) error {
	if m.remoteSave == nil {
		return fmt.Errorf("remote-save 未装配")
	}
	if _, ok := m.getTask(taskID); !ok {
		return fmt.Errorf("任务不存在: %s", taskID)
	}
	return m.remoteSave.TriggerTask(taskID, force)
}

// RemoteSaveStatus 返回任务级远程保存聚合状态（同时包含任务配置视图）。
func (m *Manager) RemoteSaveStatus(taskID string) (map[string]any, error) {
	t, ok := m.getTask(taskID)
	if !ok {
		return nil, fmt.Errorf("任务不存在: %s", taskID)
	}
	cfg, err := m.remoteSaveTaskConfig(taskID)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"task_id":     taskID,
		"enabled":     cfg.Enabled,
		"volume":      cfg.Volume,
		"dir":         cfg.Dir,
		"object_dirs": cfg.ObjectDirs,
	}
	if !cfg.Enabled {
		return out, nil
	}
	stats := map[string]int{}
	var conflicts int
	objs, err := m.listRemoteSaveObjects(t)
	if err != nil {
		return nil, err
	}
	objectsTotal := 0
	for _, obj := range objs {
		rs := obj.GetRemoteSave()
		if rs == nil {
			if obj.GetStatus() == model.StatusCompleted {
				stats["pending"]++
				objectsTotal++
			}
			continue
		}
		objectsTotal++
		stats[rs.Status]++
		conflicts += rs.Conflicts
	}
	out["objects_total"] = objectsTotal
	out["statuses"] = stats
	if m.remoteSave != nil {
		conflicts = len(m.remoteSave.ListConflicts(taskID))
	}
	out["conflicts"] = conflicts
	return out, nil
}

// ResolveRemoteSaveConflict 对单个冲突文件执行人工处理动作。
func (m *Manager) ResolveRemoteSaveConflict(ctx context.Context, taskID, objectURL, rel, action string) error {
	if m.remoteSave == nil {
		return fmt.Errorf("remote-save 未装配")
	}
	return m.remoteSave.ResolveConflict(ctx, taskID, objectURL, rel, action)
}

// PlayRemoteURL 生成/刷新对象的签名播放 URL（有有效期）。
func (m *Manager) PlayRemoteURL(taskID, objectURL string) (string, time.Time, error) {
	t, ok := m.getTask(taskID)
	if !ok {
		return "", time.Time{}, fmt.Errorf("任务不存在: %s", taskID)
	}
	obj, err := t.Storage().Get(objectURL)
	if err != nil || obj == nil {
		return "", time.Time{}, fmt.Errorf("对象不存在: %s", objectURL)
	}
	if m.remoteSave == nil {
		return "", time.Time{}, fmt.Errorf("remote-save 未装配")
	}
	return m.remoteSave.PlayURL(t, obj)
}

// RemoteSaveConflicts 返回任务下所有逐文件冲突（直接读 manifest，不受 mongo Files 截断影响）。
func (m *Manager) RemoteSaveConflicts(taskID string) ([]map[string]any, error) {
	if _, ok := m.getTask(taskID); !ok {
		return nil, fmt.Errorf("任务不存在: %s", taskID)
	}
	if m.remoteSave == nil {
		return nil, fmt.Errorf("remote-save 未装配")
	}
	raw := m.remoteSave.ListConflicts(taskID)
	out := make([]map[string]any, 0, len(raw))
	for _, c := range raw {
		out = append(out, map[string]any{
			"object_url":    c.ObjectURL,
			"object_id":     c.ObjectID,
			"path":          c.Path,
			"remote_path":   c.RemotePath,
			"size":          c.Size,
			"sha256":        c.SHA256,
			"remote_sha256": c.RemoteSHA256,
			"reason":        c.Reason,
		})
	}
	return out, nil
}

func (m *Manager) listRemoteSaveObjects(t core.Task) ([]*model.DownloadObject, error) {
	return m.listObjectsByStatus(t, []string{model.StatusCompleted, model.StatusFailed, model.StatusFailedPermanent})
}

func (m *Manager) listObjectsByStatus(t core.Task, statuses []string) ([]*model.DownloadObject, error) {
	st := t.Storage()
	if st == nil {
		return nil, fmt.Errorf("任务无存储")
	}
	const batch = 200
	var out []*model.DownloadObject
	for off := int64(0); ; off += batch {
		objs, err := st.Search(&core.StorageQuery{
			Filter: core.StorageFilter{TaskIDs: []string{t.ID()}, Statuses: statuses},
			Offset: off,
			Limit:  batch,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
		if int64(len(objs)) < batch {
			break
		}
	}
	return out, nil
}

func (m *Manager) remoteSaveTaskConfig(taskID string) (remotesave.TaskConfig, error) {
	cfg := m.currentCfg()
	if cfg == nil {
		return remotesave.TaskConfig{}, fmt.Errorf("配置不可用")
	}
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == taskID {
			return remotesave.ParseTaskConfig(cfg.Tasks[i].Extra)
		}
	}
	return remotesave.TaskConfig{}, nil
}
