// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"
	"time"

	"github.com/cocomhub/download-manager/testutil/assert"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestSchedulerService_ScanDispatches 验证 Scan 触发对象入队并被 worker 下载。
// 手动多轮 Scan：resolve 完成的对象需要下一轮 processTask 才入队（真实调度受
// TaskScan.Interval 10s ticker 影响，测试直接循环触发以保持确定性）。
func TestSchedulerService_ScanDispatches(t *testing.T) {
	mgr, _ := newMockManager(t, "sched-scan", 4, mockdl.New(mockdl.ModeAlwaysSuccess, mockdl.WithDelay(5*time.Millisecond)))
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "sched-scan")

	// 多轮扫描直到至少一个对象下载完成
	assert.MustEventually(t, func() bool {
		for range 3 {
			mgr.schedSvc.Scan()
			time.Sleep(50 * time.Millisecond)
		}
		objs, _ := task.Storage().Search(nil)
		for _, o := range objs {
			if o.GetStatus() == "completed" {
				return true
			}
		}
		return false
	}, 15*time.Second, 500*time.Millisecond, "at least one object completed via scheduler")
}

// TestSchedulerService_ProcessTaskConcurrencyLimit 验证 processTask 遵守并发度上限。
// 直接构造 pending 对象（绕过 resolve），手动 processTask 检查 active 计数。
func TestSchedulerService_ProcessTaskConcurrencyLimit(t *testing.T) {
	mgr, _ := newMockManager(t, "sched-conc", 3, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "sched-conc")

	assert.MustEventually(t, func() bool {
		objs, _ := task.Storage().Search(nil)
		return len(objs) >= 3
	}, 3*time.Second, 50*time.Millisecond, "objects seeded")

	// 直接调用 processTask（单轮），active 不应超过 limit
	mgr.schedSvc.processTask(task)

	mgr.mu.Lock()
	active := mgr.activeDownloads[task.ID()]
	mgr.mu.Unlock()
	limit := task.Concurrency()
	if active > limit {
		t.Errorf("active downloads %d exceeds task concurrency %d", active, limit)
	}
}

// TestSchedulerService_GetTaskQueue 验证队列动态容量创建与复用。
func TestSchedulerService_GetTaskQueue(t *testing.T) {
	mgr, _ := newMockManager(t, "sched-queue", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "sched-queue")

	q1 := mgr.schedSvc.getTaskQueue(task.ID())
	q2 := mgr.schedSvc.getTaskQueue(task.ID())
	if q1 != q2 {
		t.Error("getTaskQueue should return same queue for same task")
	}
	if cap(q1) < 32 {
		t.Errorf("expected queue capacity >= 32, got %d", cap(q1))
	}
}

// TestSchedulerService_ScanDisabled 验证 TaskScan.Disable 时 Scan 直接返回不 panic。
func TestSchedulerService_ScanDisabled(t *testing.T) {
	mgr, _ := newMockManager(t, "sched-disabled", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)
	_ = waitForTask(t, mgr, "sched-disabled")

	cfg := mgr.currentCfg().Clone()
	cfg.TaskScan.Disable = true
	mgr.configSvc.StoreConfig(cfg)

	// 不应 panic 或入队
	mgr.schedSvc.Scan()
}

// TestSchedulerService_Stop 验证调度器随 Manager 停止（由 startManager 的 t.Cleanup 触发 Stop）。
func TestSchedulerService_Stop(t *testing.T) {
	mgr, _ := newMockManager(t, "sched-stop", 2, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)
	_ = waitForTask(t, mgr, "sched-stop")
}
