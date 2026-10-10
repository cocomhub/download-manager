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

// SetObjectCloudDownload 设置单个下载对象的「云端下载」选项（任务自行管理的下载项级开关）。
// 仅影响后续调度：标记后由 Manager.selectDownloader 路由到 sproxy_cloud 下载器。
func (oc *ObjectController) SetObjectCloudDownload(taskID, url string, enabled bool) error {
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
	obj.SetCloudDownload(enabled)
	if err := t.Storage().Update(obj); err != nil {
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

// RetryObjectsBatch 批量重试失败对象（status ∈ failed / failed_permanent → pending）。
// 返回 URL → 结果字符串（"ok" 或错误信息），供前端逐条展示。
func (oc *ObjectController) RetryObjectsBatch(taskID string, urls []string) map[string]string {
	m := oc.m
	res := make(map[string]string, len(urls))
	t, ok := m.getTask(taskID)
	if !ok {
		for _, u := range urls {
			res[u] = errTaskNotFound.Error()
		}
		return res
	}
	count := 0
	for _, u := range urls {
		obj, err := m.getTaskObject(t, u)
		if err != nil {
			res[u] = err.Error()
			continue
		}
		if obj == nil {
			res[u] = "object not found"
			continue
		}
		st := obj.GetStatus()
		if st != model.StatusFailed && st != model.StatusFailedPermanent {
			res[u] = fmt.Sprintf("object status is %s, only failed objects can be retried", st)
			continue
		}
		// 用 SetStatusUnlessCancelled 原子重置：避免并发 CancelObject 已置 cancelled
		// 后仍被此处覆盖回 pending（对象应在取消后保留 cancelled 状态）。
		reset := false
		if guard, ok := t.(core.TaskStatusGuarder); ok {
			reset = guard.SetStatusUnlessCancelled(obj, model.StatusPending, nil)
		} else {
			t.UpdateStatus(obj, model.StatusPending, nil)
			reset = true
		}
		if !reset {
			res[u] = "object was cancelled, not retried"
			continue
		}
		obj.SetProgress(0)
		m.getOrCreateMetrics(t.ID()).retried.Add(1)
		res[u] = "ok"
		count++
	}
	if count > 0 {
		select {
		case m.schedulerSignal <- struct{}{}:
		default:
		}
	}
	return res
}

// DeleteObjectsBatch 批量删除对象：从存储与共享注册表移除，并级联清理
// runtime 下载槽位、失败计数、进度缓存、inflight 跟踪与共享缓存。
func (oc *ObjectController) DeleteObjectsBatch(taskID string, urls []string) map[string]string {
	m := oc.m
	res := make(map[string]string, len(urls))
	t, ok := m.getTask(taskID)
	if !ok {
		for _, u := range urls {
			res[u] = errTaskNotFound.Error()
		}
		return res
	}
	st := t.Storage()
	if st == nil {
		for _, u := range urls {
			res[u] = "task has no storage"
		}
		return res
	}
	for _, u := range urls {
		// 若对象正在下载：取消并释放下载槽位，避免删除后仍占用。
		oc.cancelActiveDownload(taskID, u)
		if err := st.Delete(u); err != nil {
			res[u] = err.Error()
			continue
		}
		if m.urlRegistry != nil {
			_ = m.urlRegistry.Delete(u)
		}
		m.failedCount.Delete(u)
		m.lastProgress.Delete(u)
		m.inflight.Delete(u)
		m.downloadingObj.Delete(u)
		res[u] = "ok"
	}
	m.BroadcastTaskUpdate(taskID)
	return res
}

// cancelActiveDownload 若 URL 正在下载则取消并释放任务下载槽位。
func (oc *ObjectController) cancelActiveDownload(taskID, url string) {
	m := oc.m
	if _, active := m.downloadingObj.Load(url); !active {
		return
	}
	if c, ok := m.getDownloader().(interface {
		Cancel(url string) error
	}); ok {
		_ = c.Cancel(url)
	}
	m.downloadingObj.Delete(url)
	m.mu.Lock()
	if m.activeDownloads[taskID] > 0 {
		m.activeDownloads[taskID]--
	}
	m.mu.Unlock()
}

// ReorderObjectsBatch 按有序 URL 列表批量重排任务对象顺序。
// 优先调用任务的 SetObjectOrder（整批原子提交）；若任务只支持单条
// SetObjectIndex，则按 urls 顺序逐条移动。
func (oc *ObjectController) ReorderObjectsBatch(taskID string, urls []string) error {
	t, ok := oc.m.getTask(taskID)
	if !ok {
		return fmt.Errorf("%w", errTaskNotFound)
	}
	if len(urls) < 2 {
		return fmt.Errorf("reorder requires at least 2 urls")
	}
	if batch, ok := t.(interface {
		SetObjectOrder(urls []string) error
	}); ok {
		return batch.SetObjectOrder(urls)
	}
	if single, ok := t.(interface {
		SetObjectIndex(url string, newIndex int) error
	}); ok {
		for i, u := range urls {
			if err := single.SetObjectIndex(u, i); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("task does not support reordering")
}
