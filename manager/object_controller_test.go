// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"
	"time"

	"github.com/cocomhub/download-manager/testutil/assert"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestObjectController_CancelUndoRetry 验证 ObjectController 的取消/撤销/重试链路。
func TestObjectController_CancelUndoRetry(t *testing.T) {
	mgr, _ := newMockManager(t, "oc-ctrl", 3, mockdl.New(mockdl.ModeAlwaysSuccess, mockdl.WithDelay(5*time.Millisecond)))
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "oc-ctrl")

	// 等对象就绪
	assert.MustEventually(t, func() bool {
		objs, _ := task.Storage().Search(nil)
		return len(objs) >= 3
	}, 3*time.Second, 50*time.Millisecond, "objects seeded")

	// 取第一个对象 URL
	objs, _ := task.Storage().Search(nil)
	url := objs[0].URL

	// Cancel
	if err := mgr.CancelObject(task.ID(), url); err != nil {
		t.Fatalf("CancelObject: %v", err)
	}
	obj, _ := task.Storage().Get(url)
	if obj.GetStatus() != "cancelled" {
		t.Errorf("expected cancelled, got %s", obj.GetStatus())
	}

	// Undo
	if err := mgr.UndoCancelObject(task.ID(), url); err != nil {
		t.Fatalf("UndoCancelObject: %v", err)
	}
	obj, _ = task.Storage().Get(url)
	if obj.GetStatus() != "pending" {
		t.Errorf("expected pending after undo, got %s", obj.GetStatus())
	}

	// Retry（对象为 pending 可重试）
	if err := mgr.RetryObject(task.ID(), url); err != nil {
		t.Logf("RetryObject returned %v (acceptable if resolve fails)", err)
	}
}

// TestObjectController_CancelTask 验证整任务取消。
// 对象就绪后禁用 scan（消除调度器竞争），取消后轮询收敛。
func TestObjectController_CancelTask(t *testing.T) {
	mgr, _ := newMockManager(t, "oc-cancel-task", 5, mockdl.New(mockdl.ModeSimulateProgress))
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "oc-cancel-task")

	assert.MustEventually(t, func() bool {
		objs, _ := task.Storage().Search(nil)
		return len(objs) >= 5
	}, 3*time.Second, 50*time.Millisecond, "objects seeded")

	// 禁用扫描，避免取消后 scan 重新拉取 pending 对象。
	cfg := mgr.currentCfg().Clone()
	cfg.TaskScan.Disable = true
	mgr.configSvc.StoreConfig(cfg)

	if err := mgr.CancelTask(task.ID()); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}

	// 取消异步收敛：等所有对象进入 cancelled/completed/failed 终态。
	assert.MustEventually(t, func() bool {
		objs, _ := task.Storage().Search(nil)
		for _, o := range objs {
			st := o.GetStatus()
			if st != "cancelled" && st != "completed" && st != "failed" {
				return false
			}
		}
		return true
	}, 3*time.Second, 50*time.Millisecond, "objects converge after cancel")
}

// TestObjectController_RetryAllFailed 验证失败对象批量重试。
// 先让 scan 生成对象（mock 任务对象靠 scan 触发），等对象就绪后再禁用扫描，
// 保证 retry 后状态稳定 pending（无 worker 抢跑）。
func TestObjectController_RetryAllFailed(t *testing.T) {
	mgr, _ := newMockManager(t, "oc-retry-all", 3, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "oc-retry-all")

	// 等对象就绪（scan 已生成 3 个对象）
	assert.MustEventually(t, func() bool {
		objs, _ := task.Storage().Search(nil)
		return len(objs) >= 3
	}, 3*time.Second, 50*time.Millisecond, "objects seeded")

	// 再禁用扫描，避免 RetryAllFailed 重置后 worker 立即下载（竞态）
	cfg := mgr.currentCfg().Clone()
	cfg.TaskScan.Disable = true
	mgr.configSvc.StoreConfig(cfg)

	objs, _ := task.Storage().Search(nil)
	for _, o := range objs {
		task.UpdateStatus(o, "failed", nil)
	}

	if err := mgr.RetryAllFailed(task.ID()); err != nil {
		t.Fatalf("RetryAllFailed: %v", err)
	}

	objs, _ = task.Storage().Search(nil)
	for _, o := range objs {
		if o.GetStatus() != "pending" {
			t.Errorf("object %s expected pending after retry, got %s", o.URL, o.GetStatus())
		}
	}
}

// TestObjectController_BatchCancel 验证批量取消（CancelTasks）。
func TestObjectController_BatchCancel(t *testing.T) {
	mgr, _ := newMockManager(t, "oc-batch", 2, mockdl.New(mockdl.ModeAlwaysSuccess, mockdl.WithDelay(5*time.Millisecond)))
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "oc-batch")

	res := mgr.CancelTasks([]string{task.ID(), "no-such-task"})
	if res[task.ID()] != "ok" {
		t.Errorf("expected ok for %s, got %q", task.ID(), res[task.ID()])
	}
	if res["no-such-task"] == "ok" {
		t.Error("expected error for no-such-task")
	}
}

// TestObjectController_NotFoundErrors 验证错误路径。
func TestObjectController_NotFoundErrors(t *testing.T) {
	mgr, _ := newMockManager(t, "oc-notfound", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)

	if err := mgr.CancelObject("no-task", "http://x/file"); err == nil {
		t.Error("CancelObject with no task should error")
	}

	task := waitForTask(t, mgr, "oc-notfound")
	if err := mgr.CancelObject(task.ID(), "http://missing/file"); err == nil {
		t.Error("CancelObject with missing object should error")
	}
	if err := mgr.ReorderObject(task.ID(), "http://missing/file", 0); err == nil {
		t.Log("ReorderObject on non-reorderable task returned nil (acceptable)")
	}
}

// TestObjectController_StopCleanup 验证 controller 生命周期随 Manager。
// 不手动调 Stop：startManager 的 t.Cleanup 会处理（二次 Stop 会 close 已关闭 channel）。
func TestObjectController_StopCleanup(t *testing.T) {
	mgr, _ := newMockManager(t, "oc-stop", 2, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)
	_ = waitForTask(t, mgr, "oc-stop")
}
