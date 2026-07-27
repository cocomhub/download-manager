// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"
	"sort"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

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

func (m *Manager) CancelTask(taskID string) error {
	t, ok := m.getTask(taskID)
	if !ok {
		return fmt.Errorf("%w", errTaskNotFound)
	}
	objs, err := m.collectTaskObjects(t, &core.StorageQuery{}, 200)
	if err != nil {
		return err
	}
	for _, obj := range objs {
		if obj.GetStatus() == model.StatusCompleted {
			continue
		}
		t.UpdateStatus(obj, model.StatusCancelled, nil)
		m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
		m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
		if _, active := m.downloadingObj.Load(obj.URL); active {
			if c, ok := m.getDownloader().(interface {
				Cancel(url string) error
			}); ok {
				_ = c.Cancel(obj.URL)
			}
			m.downloadingObj.Delete(obj.URL)
			m.mu.Lock()
			if m.activeDownloads[taskID] > 0 {
				m.activeDownloads[taskID]--
			}
			m.mu.Unlock()
			select {
			case m.schedulerSignal <- struct{}{}:
			default:
			}
		}
	}
	m.BroadcastTaskUpdate(taskID)
	return nil
}

func (m *Manager) CancelTasks(ids []string) map[string]string {
	result := make(map[string]string)
	for _, id := range ids {
		if err := m.CancelTask(id); err != nil {
			result[id] = err.Error()
		} else {
			result[id] = "ok"
		}
	}
	return result
}

// CancelObject 取消单个对象下载（对象级别）
func (m *Manager) CancelObject(taskID, url string) error {
	t, ok := m.getTask(taskID)
	if !ok {
		return fmt.Errorf("%w", errTaskNotFound)
	}
	obj, err := m.getTaskObject(t, url)
	if err != nil {
		return err
	}
	if obj == nil {
		return fmt.Errorf("object not found")
	}
	if obj.GetStatus() == model.StatusCompleted {
		return fmt.Errorf("object already completed, use delete to remove it")
	}
	t.UpdateStatus(obj, model.StatusCancelled, nil)
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
	m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
	if _, active := m.downloadingObj.Load(obj.URL); active {
		if c, ok := m.getDownloader().(interface {
			Cancel(url string) error
		}); ok {
			_ = c.Cancel(obj.URL)
		}
		m.downloadingObj.Delete(obj.URL)
		m.mu.Lock()
		if m.activeDownloads[taskID] > 0 {
			m.activeDownloads[taskID]--
		}
		m.mu.Unlock()
		select {
		case m.schedulerSignal <- struct{}{}:
		default:
		}
	}
	m.BroadcastTaskUpdate(taskID)
	return nil
}

// UndoCancelObject 撤销取消，将对象恢复为待下载
func (m *Manager) UndoCancelObject(taskID, url string) error {
	t, ok := m.getTask(taskID)
	if !ok {
		return fmt.Errorf("%w", errTaskNotFound)
	}
	obj, err := m.getTaskObject(t, url)
	if err != nil {
		return err
	}
	if obj == nil {
		return fmt.Errorf("object not found")
	}
	if obj.GetStatus() != model.StatusCancelled {
		return fmt.Errorf("object status is not cancelled")
	}
	t.UpdateStatus(obj, model.StatusPending, nil)
	obj.SetProgress(0)
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
	m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
	// 通知调度器：不要直接调用 processTask，会绕过 processingTask 守卫
	select {
	case m.schedulerSignal <- struct{}{}:
	default:
	}
	m.BroadcastTaskUpdate(taskID)
	return nil
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

func (m *Manager) ReorderObject(taskID, url string, newIndex int) error {
	t, ok := m.getTask(taskID)

	if !ok {
		return fmt.Errorf("%w", errTaskNotFound)
	}

	if st, ok := t.(interface {
		SetObjectIndex(url string, newIndex int) error
	}); ok {
		return st.SetObjectIndex(url, newIndex)
	}
	return fmt.Errorf("task does not support reordering")
}

// UpdateObjectTags 更新指定下载对象的标签。
func (m *Manager) UpdateObjectTags(taskType string, id int64, tags []string) error {
	task := m.FirstTaskOfType(taskType)
	if task == nil {
		return fmt.Errorf("%w: task type %q not found", errTaskNotFound, taskType)
	}
	obj, err := m.GetObjectByTypeAndID(taskType, id)
	if err != nil {
		return err
	}
	if obj == nil {
		return fmt.Errorf("object not found by type %q and id %d", taskType, id)
	}
	obj.SetTags(tags)
	if err := task.Storage().Update(obj); err != nil {
		return err
	}
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
	m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
	return nil
}
