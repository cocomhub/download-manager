// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/testutil/assert"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestDrain_InFlightCompletesNotCancelled 排空语义：触发 drain 后，
// 正在下载的对象不被取消（照常完成），排队对象不再被下载（保持 pending）。
func TestDrain_InFlightCompletesNotCancelled(t *testing.T) {
	mgr, _ := newMockManager(t, "drain-test", 3,
		mockdl.New(mockdl.ModeAlwaysSuccess, mockdl.WithDelay(200*time.Millisecond)))

	done := make(chan struct{})
	go func() {
		mgr.Start()
		close(done)
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	select {
	case <-mgr.Initialized():
	case <-ctx.Done():
		t.Fatal("manager failed to initialize")
	}

	var stopped atomic.Bool
	stopOnce := func() {
		if stopped.Load() {
			return
		}
		stopped.Store(true)
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		mgr.Stop(stopCtx)
		<-done
	}
	defer stopOnce()

	task := waitForTask(t, mgr, "drain-test")

	// 等待至少一个对象开始下载（inflight 非空）。
	assert.MustEventually(t, func() bool {
		mgr.scan()
		return mgr.countInflight() > 0
	}, 10*time.Second, 100*time.Millisecond, "expected at least one in-flight download")

	// 触发排空：不取消在途，只停新任务。
	if !mgr.StartDrain() {
		t.Fatal("StartDrain should return true on first call")
	}
	// 幂等：二次调用返回 false。
	if mgr.StartDrain() {
		t.Fatal("StartDrain should return false when already draining")
	}

	// 在途对象应最终完成（completed），而非被标 failed。
	assert.MustEventually(t, func() bool {
		all := getAllObjectsFromTask(t, task)
		completed := 0
		for _, obj := range all {
			if obj.GetStatus() == model.StatusCompleted {
				completed++
			}
		}
		return completed >= 1
	}, 10*time.Second, 200*time.Millisecond, "in-flight download should complete during drain")

	// 排空结束通知应关闭（在途耗尽）。
	assert.MustEventually(t, func() bool {
		select {
		case <-mgr.DrainDone():
			return true
		default:
			return false
		}
	}, 10*time.Second, 100*time.Millisecond, "DrainDone should close when in-flight finishes")

	// 排队对象应已回收为 pending（本次不下载，下次续跑）。
	// 在途对象可能仍在 drain 完成时处于 downloading→completed 的过渡态，
	// 也接受 downloading（稍后完成）；这里只断言没有 failed（未被取消）。
	all := getAllObjectsFromTask(t, task)
	for _, obj := range all {
		switch obj.GetStatus() {
		case model.StatusCompleted, model.StatusPending, model.StatusDownloading:
		default:
			t.Errorf("object %s status = %s, want completed/pending/downloading", obj.URL, obj.GetStatus())
		}
	}
}

// TestDrain_NoNewTasksAfterDrain 排空模式：drain 后 scan 不再入队新对象，
// forceDownload 不再发起新下载。
func TestDrain_NoNewTasksAfterDrain(t *testing.T) {
	mgr, _ := newMockManager(t, "drain-null", 5,
		mockdl.New(mockdl.ModeAlwaysSuccess, mockdl.WithDelay(10*time.Millisecond)))

	done := make(chan struct{})
	go func() {
		mgr.Start()
		close(done)
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	select {
	case <-mgr.Initialized():
	case <-ctx.Done():
		t.Fatal("manager failed to initialize")
	}

	var stopped atomic.Bool
	stopOnce := func() {
		if stopped.Load() {
			return
		}
		stopped.Store(true)
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		mgr.Stop(stopCtx)
		<-done
	}
	defer stopOnce()

	task := waitForTask(t, mgr, "drain-null")

	// 先等对象 seed（scan→GetDownloadObjects 产生对象），并让 drain 前的在途（若有）清空。
	var all []*model.DownloadObject
	assert.MustEventually(t, func() bool {
		mgr.scan()
		all = getAllObjectsFromTask(t, task)
		return len(all) > 0
	}, 5*time.Second, 100*time.Millisecond, "expected seeded objects")

	if !mgr.StartDrain() {
		t.Fatal("StartDrain should return true")
	}

	// drain 后 scan 不应产生新下载（没有对象进入 downloading/downloadingObj 增长）。
	mgr.scan()
	time.Sleep(300 * time.Millisecond)
	if n := mgr.countInflight(); n != 0 {
		t.Errorf("scan after drain started %d new downloads, want 0", n)
	}

	// drain 后 forceDownload 也应拒绝（不登记 downloadingObj/inflight）。
	mgr.forceDownload(task, all[0])
	time.Sleep(200 * time.Millisecond)
	if n := mgr.countInflight(); n != 0 {
		t.Errorf("forceDownload after drain started %d new downloads, want 0", n)
	}
}

// TestDrain_AlreadyDrainingReturnsFalse 幂等：重复触发只生效一次。
func TestDrain_AlreadyDrainingReturnsFalse(t *testing.T) {
	mgr := NewManager(&config.Config{Server: config.Server{WorkDir: t.TempDir()}})
	if !mgr.StartDrain() {
		t.Fatal("first StartDrain should succeed")
	}
	if mgr.StartDrain() {
		t.Fatal("second StartDrain should fail")
	}
	if !mgr.DrainRequested() {
		t.Fatal("DrainRequested should be true after trigger")
	}
}

// TestDrain_NoInflightCompletesImmediately 无在途下载时触发排空，
// DrainDone 应立刻关闭，避免 WaitForDrain 永久阻塞
// （drainDone 唯一关闭点是 download() defer，触发时若无 download() 在跑，
// 该 defer 永不执行 → 曾导致 SIGINT 也救不回）。
func TestDrain_NoInflightCompletesImmediately(t *testing.T) {
	mgr := NewManager(&config.Config{Server: config.Server{WorkDir: t.TempDir()}})

	if !mgr.StartDrain() {
		t.Fatal("StartDrain should return true on first call")
	}

	// StartDrain 返回后，DrainDone 应已关闭（或很快关闭）。
	select {
	case <-mgr.DrainDone():
	default:
		// 留一点窗口再用轮询兜底，避免调度/信号竞态造成的偶发。
		ctx, cancel := context.WithTimeout(t.Context(), 1*time.Second)
		defer cancel()
		select {
		case <-mgr.DrainDone():
		case <-ctx.Done():
			t.Fatal("DrainDone should close immediately when no in-flight downloads")
		}
	}
}
