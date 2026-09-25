// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"
	"sort"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// SeedTaskObjects 向已装载任务注入预构建对象（测试/fixture 用）。
// 框架测试能力：供外部（如 sdserver e2e fixture）构造模拟数据，不触发任务抓取。
func (m *Manager) SeedTaskObjects(id string, objs []*model.DownloadObject) error {
	t, ok := m.getTask(id)
	if !ok {
		return fmt.Errorf("%w: %s", errTaskNotFound, id)
	}
	st := t.Storage()
	if st == nil {
		return fmt.Errorf("task %s has no storage", id)
	}
	typ := t.Type()
	for _, o := range objs {
		o.TaskID = id
		o.EnsureTaskType(typ)
		if err := st.Update(o); err != nil {
			return fmt.Errorf("seed %s: %w", o.URL, err)
		}
	}
	return nil
}

func (m *Manager) getTask(id string) (core.Task, bool) {
	if v, ok := m.tasks.Load(id); ok {
		return v.(core.Task), true
	}
	return nil, false
}

func (m *Manager) getTaskObject(t core.Task, url string) (*model.DownloadObject, error) {
	list, err := m.searchTaskObjects(t, &core.StorageQuery{
		Filter: core.StorageFilter{
			URLs: []string{url},
		},
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}

// CancelTask 取消任务下所有未完成对象（委托 ObjectController）。
func (m *Manager) CancelTask(taskID string) error {
	return m.objectCtrl.CancelTask(taskID)
}

// CancelTasks 批量取消多个任务（委托 ObjectController）。
func (m *Manager) CancelTasks(ids []string) map[string]string {
	return m.objectCtrl.CancelTasks(ids)
}

// CancelObject 取消单个对象下载（委托 ObjectController）。
func (m *Manager) CancelObject(taskID, url string) error {
	return m.objectCtrl.CancelObject(taskID, url)
}

// UndoCancelObject 撤销取消，将对象恢复为待下载（委托 ObjectController）。
func (m *Manager) UndoCancelObject(taskID, url string) error {
	return m.objectCtrl.UndoCancelObject(taskID, url)
}

// UniqueTaskTypes 返回所有已注册任务的不重复类型列表。
func (m *Manager) UniqueTaskTypes() []string {
	seen := make(map[string]bool)
	var types []string
	m.tasks.Range(func(_, value any) bool {
		t := value.(core.Task)
		tt := t.Type()
		if !seen[tt] {
			seen[tt] = true
			types = append(types, tt)
		}
		return true
	})
	return types
}

// FirstTaskOfType 返回指定类型的第一个 Task 实例。
func (m *Manager) FirstTaskOfType(taskType string) core.Task {
	var found core.Task
	m.tasks.Range(func(_, value any) bool {
		t := value.(core.Task)
		if t.Type() == taskType {
			found = t
			return false
		}
		return true
	})
	return found
}

// GetObjectByTypeAndID 按任务类型和数字 ID 查找单个下载对象。
// 返回 nil 表示未找到。
func (m *Manager) GetObjectByTypeAndID(taskType string, id int64) (*model.DownloadObject, error) {
	task := m.FirstTaskOfType(taskType)
	if task == nil {
		return nil, fmt.Errorf("%w: task type %q not found", errTaskNotFound, taskType)
	}
	st := task.Storage()
	if st == nil {
		return nil, nil
	}
	objects, err := st.Search(&core.StorageQuery{
		Filter: core.StorageFilter{
			IDs: []int64{id},
		},
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}
	if len(objects) == 0 {
		return nil, nil
	}
	return objects[0], nil
}

// GetCollectionByID 返回指定对象所在合集的所有对象。
// 按 collection_title 排序。
func (m *Manager) GetCollectionByID(taskType string, id int64) ([]*model.DownloadObject, error) {
	// 先查找对象
	obj, err := m.GetObjectByTypeAndID(taskType, id)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, nil
	}

	// 读取 collection_id
	obj.RLock()
	collectionID := obj.Metadata["collection_id"]
	obj.RUnlock()

	if collectionID == "" {
		return []*model.DownloadObject{}, nil
	}

	// 查询同一 collection 的所有对象
	task := m.FirstTaskOfType(taskType)
	if task == nil {
		return nil, fmt.Errorf("%w: task type %q not found", errTaskNotFound, taskType)
	}
	st := task.Storage()
	if st == nil {
		return nil, nil
	}

	// 使用 metadata 精确匹配
	objects, err := st.Search(&core.StorageQuery{
		Filter: core.StorageFilter{
			TaskIDs:  []string{task.ID()},
			Metadata: map[string]string{"collection_id": collectionID},
		},
		Limit: core.NoLimit,
	})
	if err != nil {
		return nil, err
	}

	// 按 collection_title 排序
	sort.Slice(objects, func(i, j int) bool {
		objects[i].RLock()
		objects[j].RLock()
		ti := objects[i].Metadata["collection_title"]
		tj := objects[j].Metadata["collection_title"]
		objects[i].RUnlock()
		objects[j].RUnlock()
		return ti < tj
	})

	return objects, nil
}

// ReorderObject 调整对象在任务中的顺序（委托 ObjectController）。
func (m *Manager) ReorderObject(taskID, url string, newIndex int) error {
	return m.objectCtrl.ReorderObject(taskID, url, newIndex)
}

// UpdateObjectTags 更新指定下载对象的标签（委托 ObjectController）。
func (m *Manager) UpdateObjectTags(taskType string, id int64, tags []string) error {
	return m.objectCtrl.UpdateObjectTags(taskType, id, tags)
}
