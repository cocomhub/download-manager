// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

// ObjectController 负责对象级与任务级的控制操作：
// 取消（单个/批量）、撤销取消、重试、重排、标签更新。
// 与 AggregationService / ConfigService 一样，作为 Manager 的职责拆分单元，
// 通过持有 Manager 引用访问共享状态（tasks、downloadingObj、schedulerSignal 等）。
type ObjectController struct {
	m *Manager
}

// newObjectController 创建对象控制器。
func newObjectController(m *Manager) *ObjectController {
	return &ObjectController{m: m}
}

// CancelTask 取消任务下所有未完成对象。
func (oc *ObjectController) CancelTask(taskID string) error {
	m := oc.m
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

// CancelTasks 批量取消多个任务，返回每个任务的结果。
func (oc *ObjectController) CancelTasks(ids []string) map[string]string {
	result := make(map[string]string)
	for _, id := range ids {
		if err := oc.CancelTask(id); err != nil {
			result[id] = err.Error()
		} else {
			result[id] = "ok"
		}
	}
	return result
}

// CancelObject 取消单个对象下载（对象级别）。
func (oc *ObjectController) CancelObject(taskID, url string) error {
	m := oc.m
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

// UndoCancelObject 撤销取消，将对象恢复为待下载。
func (oc *ObjectController) UndoCancelObject(taskID, url string) error {
	m := oc.m
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

// ReorderObject 调整对象在任务中的顺序。
func (oc *ObjectController) ReorderObject(taskID, url string, newIndex int) error {
	t, ok := oc.m.getTask(taskID)
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
func (oc *ObjectController) UpdateObjectTags(taskType string, id int64, tags []string) error {
	m := oc.m
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

// RetryObject resets the status of an object to pending and forces download。
func (oc *ObjectController) RetryObject(taskID, url string) error {
	m := oc.m
	t, ok := m.getTask(taskID)
	if !ok {
		return fmt.Errorf("%w", errTaskNotFound)
	}
	obj, err := m.getTaskObject(t, url)
	if err != nil {
		return err
	}
	if obj != nil {
		if obj.GetStatus() == model.StatusCompleted {
			return fmt.Errorf("object already completed")
		}
		// Reset status
		t.UpdateStatus(obj, model.StatusPending, nil)
		obj.SetProgress(0)

		// Resolve details if needed (JIT for forced retry?)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := t.ResolveObject(ctx, obj); err != nil {
			slog.Error("Failed to resolve object for retry", logutil.LogKeyError, err)
			return fmt.Errorf("failed to resolve object: %v", err)
		}

		m.forceDownload(t, obj)
		m.getOrCreateMetrics(t.ID()).retried.Add(1)
		return nil
	}
	return fmt.Errorf("object not found")
}

// RetryAllFailed resets all failed objects in a task。
func (oc *ObjectController) RetryAllFailed(taskID string) error {
	m := oc.m
	t, ok := m.getTask(taskID)
	if !ok {
		return fmt.Errorf("%w", errTaskNotFound)
	}
	objs, err := m.collectTaskObjects(t, &core.StorageQuery{
		Filter: core.StorageFilter{
			Statuses: []string{model.StatusFailed, model.StatusFailedPermanent},
		},
	}, 200)
	if err != nil {
		return err
	}
	count := 0
	for _, obj := range objs {
		t.UpdateStatus(obj, model.StatusPending, nil)
		obj.SetProgress(0)
		m.getOrCreateMetrics(t.ID()).retried.Add(1)
		count++
	}
	if count > 0 {
		// 通知调度器：不要直接调用 processTask，会绕过 processingTask 守卫
		select {
		case m.schedulerSignal <- struct{}{}:
		default:
		}
	}
	return nil
}
