// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"

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
	objs, err := m.listCompletedObjects(t)
	if err != nil {
		return nil, err
	}
	for _, obj := range objs {
		rs := obj.GetRemoteSave()
		if rs == nil {
			stats["pending"]++
			continue
		}
		stats[rs.Status]++
		conflicts += rs.Conflicts
	}
	out["objects_total"] = len(objs)
	out["statuses"] = stats
	out["conflicts"] = conflicts
	return out, nil
}

// RemoteSaveConflicts 返回任务下所有逐文件冲突（只读，供后续 UI 人工处理）。
func (m *Manager) RemoteSaveConflicts(taskID string) ([]map[string]any, error) {
	t, ok := m.getTask(taskID)
	if !ok {
		return nil, fmt.Errorf("任务不存在: %s", taskID)
	}
	objs, err := m.listCompletedObjects(t)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, obj := range objs {
		rs := obj.GetRemoteSave()
		if rs == nil {
			continue
		}
		for _, f := range rs.Files {
			if f.Status != model.RemoteFileStatusConflict {
				continue
			}
			out = append(out, map[string]any{
				"object_url":    obj.URL,
				"object_id":     obj.GetID(),
				"path":          f.Path,
				"remote_path":   f.RemotePath,
				"size":          f.Size,
				"sha256":        f.SHA256,
				"remote_sha256": f.RemoteSHA256,
				"reason":        f.Reason,
			})
		}
	}
	return out, nil
}

func (m *Manager) listCompletedObjects(t core.Task) ([]*model.DownloadObject, error) {
	st := t.Storage()
	if st == nil {
		return nil, fmt.Errorf("任务无存储")
	}
	const batch = 200
	var out []*model.DownloadObject
	for off := int64(0); ; off += batch {
		objs, err := st.Search(&core.StorageQuery{
			Filter: core.StorageFilter{TaskIDs: []string{t.ID()}, Statuses: []string{model.StatusCompleted}},
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
