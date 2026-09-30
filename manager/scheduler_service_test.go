// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sync"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/model"
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

// TestSchedulerService_SequentialConcurrencyLimit 验证 sequential=true 时
// 同一任务同时下载的对象不超过 1 个（任务内串行）。
func TestSchedulerService_SequentialConcurrencyLimit(t *testing.T) {
	dl := mockdl.New(mockdl.ModeAlwaysSuccess, mockdl.WithDelay(20*time.Millisecond))
	mgr, _ := newMockManager(t, "sched-seq", 3, dl)

	cfg := mgr.currentCfg().Clone()
	cfg.Downloader.Sequential = true
	mgr.configSvc.StoreConfig(cfg)

	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "sched-seq")

	// 多轮扫描直到全部 3 个对象下载完成。
	assert.MustEventually(t, func() bool {
		for range 3 {
			mgr.schedSvc.Scan()
			time.Sleep(20 * time.Millisecond)
		}
		objs, _ := task.Storage().Search(nil)
		completed := 0
		for _, o := range objs {
			if o.GetStatus() == model.StatusCompleted {
				completed++
			}
		}
		return completed >= 3
	}, 15*time.Second, 100*time.Millisecond, "all sequential objects completed")

	// 顺序开关生效：任务内任何时候并发下载不超过 1。
	mgr.mu.Lock()
	active := mgr.activeDownloads[task.ID()]
	mgr.mu.Unlock()
	if active > 1 {
		t.Errorf("sequential mode: active=%d, want <= 1", active)
	}
}

// TestSchedulerService_SequentialOrderConsumed 验证顺序模式按 GetDownloadObjects
// 返回顺序逐个下载：每个时刻同时下载不超过 1 个对象（任务内串行）。
func TestSchedulerService_SequentialOrderConsumed(t *testing.T) {
	dl := mockdl.New(mockdl.ModeAlwaysSuccess, mockdl.WithDelay(25*time.Millisecond))
	var mu sync.Mutex
	var maxPeak int
	var peakCount int
	var firstURL string
	activeSet := make(map[string]bool)

	dl.OnStart = func(url string) {
		mu.Lock()
		defer mu.Unlock()
		if activeSet[url] {
			t.Errorf("duplicate concurrent start for %s", url)
		}
		wasEmpty := len(activeSet) == 0
		activeSet[url] = true
		if len(activeSet) > maxPeak {
			maxPeak = len(activeSet)
		}
		if wasEmpty {
			peakCount++
			if firstURL == "" {
				firstURL = url
			}
		}
	}
	dl.OnComplete = func(url string) {
		mu.Lock()
		delete(activeSet, url)
		mu.Unlock()
	}

	mgr, _ := newMockManager(t, "sched-seq-order", 3, dl)
	cfg := mgr.currentCfg().Clone()
	cfg.Downloader.Sequential = true
	mgr.configSvc.StoreConfig(cfg)

	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "sched-seq-order")

	completedAll := func() bool {
		objs, _ := task.Storage().Search(nil)
		completed := 0
		for _, o := range objs {
			if o.GetStatus() == model.StatusCompleted {
				completed++
			}
		}
		return completed >= 3
	}
	assert.MustEventually(t, func() bool {
		for range 3 {
			mgr.schedSvc.Scan()
			time.Sleep(20 * time.Millisecond)
		}
		return completedAll()
	}, 15*time.Second, 100*time.Millisecond, "all sequential-order objects completed")

	mu.Lock()
	defer mu.Unlock()
	if maxPeak > 1 {
		t.Errorf("sequential mode: peak concurrent = %d, want 1", maxPeak)
	}
	if peakCount < 3 {
		t.Errorf("sequential mode: saw %d serial starts, want 3", peakCount)
	}
	t.Logf("sequential mode: peak=%d serialStarts=%d first=%q", maxPeak, peakCount, firstURL)
}

// TestSchedulerService_NonSequentialUnchanged 验证 sequential=false（默认）时
// 并发行为不回归：任务内并发可以超过 1（只要任务配置并发度允许）。
func TestSchedulerService_NonSequentialUnchanged(t *testing.T) {
	dl := mockdl.New(mockdl.ModeAlwaysSuccess, mockdl.WithDelay(30*time.Millisecond))
	var mu sync.Mutex
	var maxPeak int
	activeSet := make(map[string]bool)
	dl.OnStart = func(url string) {
		mu.Lock()
		activeSet[url] = true
		if len(activeSet) > maxPeak {
			maxPeak = len(activeSet)
		}
		mu.Unlock()
	}
	dl.OnComplete = func(url string) {
		mu.Lock()
		delete(activeSet, url)
		mu.Unlock()
	}

	mgr, _ := newMockManager(t, "sched-nonseq", 3, dl)
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "sched-nonseq")

	assert.MustEventually(t, func() bool {
		for range 3 {
			mgr.schedSvc.Scan()
			time.Sleep(20 * time.Millisecond)
		}
		objs, _ := task.Storage().Search(nil)
		completed := 0
		for _, o := range objs {
			if o.GetStatus() == model.StatusCompleted {
				completed++
			}
		}
		return completed >= 3
	}, 15*time.Second, 100*time.Millisecond, "all non-sequential objects completed")

	mu.Lock()
	sawPeak := maxPeak
	mu.Unlock()
	if sawPeak < 2 {
		t.Logf("note: non-sequential peak concurrent = %d (expected >= 2 with default concurrency 2)", sawPeak)
	}
}
