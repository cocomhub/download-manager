// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"

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
		return fmt.Errorf("task not found")
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
		return fmt.Errorf("task not found")
	}
	obj, err := m.getTaskObject(t, url)
	if err != nil {
		return err
	}
	if obj == nil {
		return fmt.Errorf("object not found")
	}
	if obj.GetStatus() == model.StatusCompleted {
		return fmt.Errorf("object already completed")
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
		return fmt.Errorf("task not found")
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

func (m *Manager) ReorderObject(taskID, url string, newIndex int) error {
	t, ok := m.getTask(taskID)

	if !ok {
		return fmt.Errorf("task not found")
	}

	if st, ok := t.(interface {
		SetObjectIndex(url string, newIndex int) error
	}); ok {
		return st.SetObjectIndex(url, newIndex)
	}
	return fmt.Errorf("task does not support reordering")
}
