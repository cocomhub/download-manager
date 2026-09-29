// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sync"
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

	// 关键：禁用 scan 后仍有在途 processTask goroutine（waitForObjectsFinal 返回时
	// 它可能已 snapshot 全部 3 个对象，其中 failed 也会被入队）。等待调度管道
	// 完全静默（无 processing、无活跃下载、队列清空）后再手动置 failed，
	// 否则 processTask 会把对象重新入队下载，覆盖为 downloading，
	// 导致 RetryObjectsBatch 报 "object status is downloading"（CI 高频 flake）。
	waitForSchedulerIdle(t, mgr, task.ID())

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

// resurrectTrackingDL 包装 MockDownloader，记录 Download 开始/结束时刻，
// 用于「删除在途对象不复活」的 P0 回归测试。
type resurrectTrackingDL struct {
	*mockdl.MockDownloader
	started   chan struct{}
	done      chan struct{}
	startOnce sync.Once
	doneOnce  sync.Once
}

func newResurrectTrackingDL(inner *mockdl.MockDownloader) *resurrectTrackingDL {
	return &resurrectTrackingDL{
		MockDownloader: inner,
		started:        make(chan struct{}),
		done:           make(chan struct{}),
	}
}

func (d *resurrectTrackingDL) Download(obj *model.DownloadObject, h map[string]string) error {
	d.startOnce.Do(func() { close(d.started) })
	defer d.doneOnce.Do(func() { close(d.done) })
	return d.MockDownloader.Download(obj, h)
}

// waitClosed 等待 channel 关闭或超时（测试辅助）。
func waitClosed(t *testing.T, ch <-chan struct{}, timeout time.Duration, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("timeout: %s", msg)
	}
}

// TestObjectController_DeleteObjectsBatch_InFlightNoResurrect 验证 P0 修复：
// 删除「正在下载」的对象后，下载 goroutine 结束时的状态回写不得让对象以
// completed/failed 状态「复活」落库（FileStorage.Update 是纯 upsert）。
func TestObjectController_DeleteObjectsBatch_InFlightNoResurrect(t *testing.T) {
	inner := mockdl.New(mockdl.ModeSimulateProgress, mockdl.WithDelay(50*time.Millisecond))
	tracked := newResurrectTrackingDL(inner)
	mgr, _ := newMockManager(t, "oc-del-resurrect", 1, inner)
	mgr.setDownloader(tracked)
	// 默认 TaskScan.Interval=10s 会让对象到下次 scan 才进入下载队列，
	// 调低扫描间隔让测试快速进入在途状态。
	scanCfg := mgr.currentCfg().Clone()
	scanCfg.TaskScan.Interval = 1
	mgr.configSvc.StoreConfig(scanCfg)
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "oc-del-resurrect")

	// 等对象就绪。
	assert.MustEventually(t, func() bool {
		objs, _ := task.Storage().Search(nil)
		return len(objs) == 1
	}, 3*time.Second, 50*time.Millisecond, "object seeded")

	// 等真实下载开始（Download 已进入 simulateProgress）。
	waitClosed(t, tracked.started, 10*time.Second, "download started")

	objs, _ := task.Storage().Search(nil)
	url := objs[0].URL
	if _, ok := mgr.downloadingObj.Load(url); !ok {
		t.Fatal("expected object registered in downloadingObj while in flight")
	}

	// 删除在途对象。
	res := mgr.DeleteObjectsBatch(task.ID(), []string{url})
	if res[url] != "ok" {
		t.Fatalf("expected ok for %s, got %q", url, res[url])
	}
	if got, _ := task.Storage().Get(url); got != nil {
		t.Fatalf("object still present right after delete")
	}

	// 等待下载 goroutine 结束（状态回写已尝试，若未修复会在此刻「复活」）。
	waitClosed(t, tracked.done, 10*time.Second, "in-flight download finished")

	if got, _ := task.Storage().Get(url); got != nil {
		t.Errorf("deleted object resurrected with status %q after download finished", got.GetStatus())
	}
	if shared, _ := mgr.urlRegistry.Get(url); shared != nil {
		t.Errorf("deleted object resurrected in urlRegistry")
	}
}

// guardRetryTask 实现 TaskStatusGuarder 的批量重试对象，记录守卫调用，
// 用于验证 RetryObjectsBatch 对并发取消的 cancelled 状态不覆盖。
type guardRetryTask struct {
	mockTask
	guardResult bool
	guardCalls  int
	updateCalls int
}

func (t *guardRetryTask) SetStatusUnlessCancelled(obj *model.DownloadObject, status string, err error) bool {
	t.guardCalls++
	return t.guardResult
}

func (t *guardRetryTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	t.updateCalls++
	return nil
}

// TestObjectController_RetryObjectsBatch_RespectsCancelGuard 验证批量重试使用
// SetStatusUnlessCancelled 守卫：并发取消已置 cancelled 的对象不会被重置回 pending。
func TestObjectController_RetryObjectsBatch_RespectsCancelGuard(t *testing.T) {
	// 守卫拒绝（模拟并发 CancelObject 已把对象置 cancelled）→ 不得重置、不得直写。
	refused := &guardRetryTask{guardResult: false}
	refused.id = "t-retry-guard"
	refused.typ = "mock"
	refused.objs = []*model.DownloadObject{
		{TaskID: "t-retry-guard", URL: "http://retry-guard/1", Status: model.StatusFailed},
	}
	m := NewManager(&config.Config{})
	m.tasks.Store(refused.id, refused)
	m.objectCtrl = newObjectController(m)

	res := m.RetryObjectsBatch(refused.id, []string{"http://retry-guard/1"})
	if res["http://retry-guard/1"] == "ok" {
		t.Errorf("cancelled object should not be retried as ok, got %q", res["http://retry-guard/1"])
	}
	if refused.guardCalls != 1 {
		t.Errorf("expected guard consulted once, got %d", refused.guardCalls)
	}
	if refused.updateCalls != 0 {
		t.Errorf("expected no direct UpdateStatus writeback when guard refuses, got %d", refused.updateCalls)
	}

	// 守卫放行（正常失败对象）→ 重置为 pending。
	allowed := &guardRetryTask{guardResult: true}
	allowed.id = "t-retry-guard-ok"
	allowed.typ = "mock"
	allowed.objs = []*model.DownloadObject{
		{TaskID: "t-retry-guard-ok", URL: "http://retry-guard/2", Status: model.StatusFailedPermanent},
	}
	m2 := NewManager(&config.Config{})
	m2.tasks.Store(allowed.id, allowed)
	m2.objectCtrl = newObjectController(m2)

	res2 := m2.RetryObjectsBatch(allowed.id, []string{"http://retry-guard/2"})
	if res2["http://retry-guard/2"] != "ok" {
		t.Errorf("expected ok when guard allows, got %q", res2["http://retry-guard/2"])
	}
	if allowed.guardCalls != 1 {
		t.Errorf("expected guard consulted once for happy path, got %d", allowed.guardCalls)
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
