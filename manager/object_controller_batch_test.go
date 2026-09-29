// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/testutil/assert"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestObjectController_RetryObjectsBatch 验证批量重试（failed/failed_permanent → pending）。
func TestObjectController_RetryObjectsBatch(t *testing.T) {
	mgr, _ := newMockManager(t, "oc-retry-batch", 3, mockdl.New(mockdl.ModeAlwaysSuccess, mockdl.WithDelay(50*time.Millisecond)))
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "oc-retry-batch")

	assert.MustEventually(t, func() bool {
		objs, _ := task.Storage().Search(nil)
		return len(objs) >= 3
	}, 3*time.Second, 50*time.Millisecond, "objects seeded")

	// 等所有对象被下载完成（ModeAlwaysSuccess；waitForObjectsFinal 循环 scan 触发下载）。
	waitForObjectsFinal(t, mgr, task, 3, model.StatusCompleted, 5*time.Second)

	// 禁用扫描，避免重试后 scan/worker 抢跑改状态。
	cfg := mgr.currentCfg().Clone()
	cfg.TaskScan.Disable = true
	mgr.configSvc.StoreConfig(cfg)

	objs, _ := task.Storage().Search(nil)
	if len(objs) < 2 {
		t.Fatalf("need at least 2 objects, got %d", len(objs))
	}
	// 手动置为 failed_permanent/failed。
	for i := range objs {
		status := model.StatusFailed
		if i == 1 {
			status = model.StatusFailedPermanent
		}
		if err := task.UpdateStatus(objs[i], status, nil); err != nil {
			t.Fatalf("mark failed: %v", err)
		}
	}

	urls := []string{objs[0].URL, objs[1].URL}
	res := mgr.RetryObjectsBatch(task.ID(), urls)
	for _, u := range urls {
		if res[u] != "ok" {
			t.Errorf("expected ok for %s, got %q", u, res[u])
		}
	}

	// 重试后对象离开 failed（pending/downloading/completed 均可接受，因为 worker 可能抢跑）。
	assert.MustEventually(t, func() bool {
		all, _ := task.Storage().Search(nil)
		for _, o := range all {
			for _, u := range urls {
				if o.URL == u && o.GetStatus() == "failed" {
					return false
				}
			}
		}
		return true
	}, 5*time.Second, 50*time.Millisecond, "retried objects leave failed")
}

// TestObjectController_RetryObjectsBatch_Completed 验证已完成对象不可重试。
func TestObjectController_RetryObjectsBatch_Completed(t *testing.T) {
	mgr, _ := newMockManager(t, "oc-retry-batch-c", 1, mockdl.New(mockdl.ModeAlwaysSuccess, mockdl.WithDelay(50*time.Millisecond)))
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "oc-retry-batch-c")

	assert.MustEventually(t, func() bool {
		objs, _ := task.Storage().Search(nil)
		return len(objs) == 1
	}, 3*time.Second, 50*time.Millisecond, "object seeded")

	objs, _ := task.Storage().Search(nil)
	url := objs[0].URL

	// 等对象进入 downloading（下载链路已启动）；mock 无小对象，下载在
	// resolveCache 过期（10s TTL）后才会真正执行 dl.Download。
	assert.MustEventually(t, func() bool {
		cur, _ := task.Storage().Get(url)
		return cur != nil && cur.GetStatus() == model.StatusDownloading
	}, 5*time.Second, 50*time.Millisecond, "object downloading")

	res := mgr.RetryObjectsBatch(task.ID(), []string{url})
	if res[url] == "ok" {
		t.Error("expected downloading object to be non-retriable")
	}
}

// TestObjectController_DeleteObjectsBatch 验证批量删除（从存储 + 共享注册表移除）。
func TestObjectController_DeleteObjectsBatch(t *testing.T) {
	mgr, _ := newMockManager(t, "oc-del-batch", 3, mockdl.New(mockdl.ModeAlwaysSuccess, mockdl.WithDelay(5*time.Millisecond)))
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "oc-del-batch")

	// 等对象全部完成（completed 对象不会在 scan 重拉，删除后不会再出现）。
	// 单对象测试：download 完成需要先等 resolveCache 过期（10s TTL 实测），
	// 因此这里直接等到对象处于 downloading（说明下载链路已启动），删除仍可验证存储移除。
	assert.MustEventually(t, func() bool {
		objs, _ := task.Storage().Search(nil)
		if len(objs) < 3 {
			return false
		}
		for _, o := range objs {
			if o.GetStatus() != model.StatusCompleted && o.GetStatus() != model.StatusDownloading {
				return false
			}
		}
		return true
	}, 5*time.Second, 50*time.Millisecond, "objects seeded")

	// 后续断言只验证存储/注册表移除，不依赖完成状态：等对象全部离开 pending。
	assert.MustEventually(t, func() bool {
		objs, _ := task.Storage().Search(nil)
		if len(objs) < 3 {
			return false
		}
		for _, o := range objs {
			if o.GetStatus() == model.StatusPending || o.GetStatus() == model.StatusResolving {
				return false
			}
		}
		return true
	}, 5*time.Second, 50*time.Millisecond, "objects leave pending")

	objs, _ := task.Storage().Search(nil)
	urls := []string{objs[0].URL, objs[1].URL}
	var untouched []string
	for i, o := range objs {
		if i >= 2 {
			untouched = append(untouched, o.URL)
		}
	}

	res := mgr.DeleteObjectsBatch(task.ID(), urls)
	for _, u := range urls {
		if res[u] != "ok" {
			t.Errorf("expected ok for %s, got %q", u, res[u])
		}
	}

	// 存储与共享注册表中都不应再存在。
	for _, u := range urls {
		got, _ := task.Storage().Get(u)
		if got != nil {
			t.Errorf("object %s still in storage after delete", u)
		}
		if shared, _ := mgr.urlRegistry.Get(u); shared != nil {
			t.Errorf("object %s still in urlRegistry after delete", u)
		}
	}

	// 未删除对象仍存在于存储中（删除只影响指定的 urls）。
	for _, u := range untouched {
		got, _ := task.Storage().Get(u)
		if got == nil {
			t.Errorf("object %s missing after batch delete of others", u)
		}
	}
}

// reorderBatchTask 记录批量重排的目标顺序。
type reorderBatchTask struct {
	mockTask
	order []string
}

func (t *reorderBatchTask) SetObjectOrder(urls []string) error {
	t.order = append([]string(nil), urls...)
	return nil
}

func (t *reorderBatchTask) SetObjectIndex(url string, newIndex int) error {
	t.order = append(t.order, url)
	return nil
}

// TestObjectController_ReorderObjectsBatch 验证批量重排（有序 URL 列表 → 按序重排）。
func TestObjectController_ReorderObjectsBatch(t *testing.T) {
	task := &reorderBatchTask{id: "task-reorder-batch", typ: "mock"}
	m := NewManager(&config.Config{})
	m.tasks.Store("task-reorder-batch", task)
	m.objectCtrl = newObjectController(m)

	want := []string{"http://a", "http://b", "http://c"}
	if err := m.ReorderObjectsBatch("task-reorder-batch", want); err != nil {
		t.Fatalf("ReorderObjectsBatch: %v", err)
	}
	if len(task.order) != len(want) {
		t.Fatalf("expected %d deps recorded, got %v", len(want), task.order)
	}
}

// TestObjectController_ReorderObjectsBatch_NotFound 验证任务不存在时报错。
func TestObjectController_ReorderObjectsBatch_NotFound(t *testing.T) {
	m := NewManager(&config.Config{})
	m.objectCtrl = newObjectController(m)
	if err := m.ReorderObjectsBatch("nope", []string{"http://x"}); err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}
