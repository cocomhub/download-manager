// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"log/slog"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

// SchedulerService 负责调度核心职责：任务扫描（scan）、对象分派（processTask）、
// 加权轮询（scheduler + recalcWeights）、任务队列管理（getTaskQueue）。
// Manager 保留生命周期协调（Start/Stop/WaitForShutdown）与 worker/resolve/so 池，
// 通过持有 Manager 引用访问共享状态。
type SchedulerService struct {
	m *Manager
}

// newSchedulerService 创建调度服务。
func newSchedulerService(m *Manager) *SchedulerService {
	return &SchedulerService{m: m}
}

// Scan 执行一次任务扫描：抓取发现新对象 + 下载分派。
func (s *SchedulerService) Scan() {
	m := s.m
	if !m.workersEnabled.Load() {
		return
	}

	if m.currentCfg().TaskScan.Disable {
		return
	}

	if !m.scanRunning.CompareAndSwap(false, true) {
		slog.Debug("scan: already running, skipping")
		return
	}
	defer m.scanRunning.Store(false)

	// Phase 1: Scrape — discover new objects from tasks that support it.
	// Run scrapes in detached goroutines with per-task ctx timeout and per-task
	// dedup guard (scrapingTask) so a slow Scrape never overlaps itself.
	// Do NOT wait — Phase 2 runs in parallel; scraped objects are persisted
	// to storage and picked up by the next scan cycle's Phase 2.
	m.tasks.Range(func(key, value any) bool {
		if sc, ok := value.(core.Scraper); ok {
			taskID := key.(string)
			t := value.(core.Task)
			// Check if this task's scrape is disabled
			taskCfg := m.findTaskConfig(t.ID())
			if taskCfg != nil && !taskCfg.GetScrapeEnabled(m.currentCfg()) {
				return true // skip scraping for this task
			}
			if _, scraping := m.scrapingTask.LoadOrStore(taskID, true); scraping {
				slog.Debug("Scrape: previous run still in progress, skipping", logutil.LogKeyTaskID, taskID)
				return true
			}
			go func(taskID string, sc core.Scraper) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				done := make(chan error, 1)
				go func() {
					done <- sc.Scrape(ctx)
				}()
				select {
				case err := <-done:
					if err != nil {
						slog.Error("Scrape failed", logutil.LogKeyTaskID, taskID, logutil.LogKeyError, err)
					}
				case <-ctx.Done():
					slog.Error("Scrape timed out", logutil.LogKeyTaskID, taskID)
					// ctx is canceled; wait for inner goroutine to actually return
					// before releasing the dedup guard, so the next scan cycle
					// does not start a second concurrent Scrape for this task.
					<-done
				}
				m.scrapingTask.Delete(taskID)
			}(taskID, sc)
		}
		return true
	})

	// Phase 2: Download — process tasks for pending objects
	tasks := make([]core.Task, 0, 64)
	m.tasks.Range(func(key, value any) bool {
		tasks = append(tasks, value.(core.Task))
		return true
	})

	for _, t := range tasks {
		// Check if task is already being processed
		if _, processing := m.processingTask.LoadOrStore(t.ID(), true); processing {
			continue
		}

		go s.processTask(t)
	}
}

// processTask 处理单个任务：按并发度约束从任务取待下载对象入队。
func (s *SchedulerService) processTask(t core.Task) {
	m := s.m
	defer m.processingTask.Delete(t.ID())

	// Check if this task's download is disabled
	taskCfg := m.findTaskConfig(t.ID())
	if taskCfg != nil && !taskCfg.GetDownloadEnabled(m.currentCfg()) {
		return
	}

	// Check per-task concurrency limit (soft limit for scheduling?)
	// If global limit is used, task limit might be redundant or acts as "fairness" limit.
	// Let's keep it.

	limit := t.Concurrency()

	m.mu.Lock()
	active := m.activeDownloads[t.ID()]
	// If active >= limit, we stop scheduling new downloads for this task.
	if active >= limit {
		m.mu.Unlock()
		// slog.Debug("Task reached concurrency limit", logutil.LogKeyTaskID, t.ID(), "active", active, "limit", limit)
		return
	}
	m.mu.Unlock()

	// Calculate remaining slots
	slotsAvailable := max(0, limit-active)

	// Only fetch objects if we have capacity
	objs, err := t.GetDownloadObjects()
	if err != nil {
		slog.Error("Error getting objects for task", logutil.LogKeyTaskID, t.ID(), logutil.LogKeyError, err)
		return
	}

	if len(objs) == 0 {
		return
	}
	// slog.Debug("Task has objects to download", logutil.LogKeyTaskID, t.ID(), "count", len(objs))

	// Schedule downloads up to available slots
	count := 0

	for _, obj := range objs {
		if count >= slotsAvailable {
			break
		}

		// 如果已经在下载队列中，跳过
		if _, loaded := m.downloadingObj.LoadOrStore(obj.URL, obj); loaded {
			continue
		}

		// 检查对象状态：需要 resolve 的异步提交
		if obj.GetStatus() == model.StatusPending && !hasFiles(obj) {
			obj.SetStatus(model.StatusResolving)
			_ = t.UpdateStatus(obj, model.StatusResolving, nil)
			m.enqueueResolve(t.ID(), obj)
			m.downloadingObj.Delete(obj.URL) // 不占用下载槽位
			continue
		}

		// 对象在 resolve 中，跳过本周期
		if obj.GetStatus() == model.StatusResolving {
			m.downloadingObj.Delete(obj.URL)
			continue
		}

		// Attempt to push to global queue
		q := m.getTaskQueue(t.ID())
		select {
		case q <- &downloadRequest{task: t, obj: obj}:
			slog.Info("Object enqueued", logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL)

			m.mu.Lock()
			m.activeDownloads[t.ID()]++
			active++
			m.mu.Unlock()
			count++

			// 通知调度器：有新的待处理对象
			select {
			case m.schedulerSignal <- struct{}{}:
			default:
			}
		default:
			// Queue full, abort scheduling for now
			// Remove from downloadingObj map since we didn't schedule it
			m.downloadingObj.Delete(obj.URL)
		}
	}
	m.BroadcastTaskUpdate(t.ID())
}

// hasFiles 检查对象是否已填充 Extra["files"]（即已 resolve 或无需 resolve）。
// 通过对象的内部锁保护 Extra map 的并发安全。
func hasFiles(obj *model.DownloadObject) bool {
	if obj == nil {
		return false
	}
	obj.RLock()
	defer obj.RUnlock()
	if obj.Extra == nil {
		return false
	}
	files, ok := obj.Extra["files"]
	if !ok {
		return false
	}
	switch f := files.(type) {
	case []any:
		return len(f) > 0
	case []map[string]string:
		return len(f) > 0
	default:
		return false
	}
}

// getTaskQueue 返回任务的下载队列（动态容量）。
func (s *SchedulerService) getTaskQueue(taskID string) chan *downloadRequest {
	m := s.m
	if v, ok := m.taskQueues.Load(taskID); ok {
		return v.(chan *downloadRequest)
	}
	// 动态容量：根据任务并发度计算，保证充分缓冲
	cap := 64 // default
	if t, ok := m.getTask(taskID); ok {
		concurrency := t.Concurrency()
		if concurrency > 0 {
			cap = max(concurrency*8, 32)
		}
	}
	cap = min(cap, 256)
	q := make(chan *downloadRequest, cap)
	if v, loaded := m.taskQueues.LoadOrStore(taskID, q); loaded {
		return v.(chan *downloadRequest)
	}
	return q
}

// scheduler 加权轮询调度器：把任务队列的请求搬移到全局下载队列。
// 500ms fallback 心跳 + schedulerSignal 事件驱动。
func (s *SchedulerService) runScheduler() {
	m := s.m
	fallbackTicker := time.NewTicker(500 * time.Millisecond)
	defer fallbackTicker.Stop()
	weights := make(map[string]int)
	lastUpdate := time.Now()

	// 快照 stop channel：scheduler goroutine 生命周期内固定引用同一 channel，
	// 避免与 Start()/Stop()/reconcileScheduler 的写形成 data race。
	m.mu.Lock()
	stopCh := m.schedulerStop
	m.mu.Unlock()
	if stopCh == nil {
		stopCh = make(chan struct{})
	}

	drainOnce := func() {
		ids := make([]string, 0, 64)
		m.tasks.Range(func(key, value any) bool {
			ids = append(ids, key.(string))
			return true
		})
		expanded := make([]string, 0, len(ids)*maxSchedulerWeight)
		for _, id := range ids {
			w := weights[id]
			if w <= 0 {
				w = 1
			}
			for i := 0; i < w; i++ {
				expanded = append(expanded, id)
			}
		}
	outerLoop:
		for _, id := range expanded {
			q := m.getTaskQueue(id)
			select {
			case req := <-q:
				select {
				case m.downloadQueue <- req:
				default:
					// global queue full, put back
					select {
					case q <- req:
					default:
						// task queue also full, drop -- next scan() will re-enqueue
					}
					break outerLoop
				}
			default:
			}
		}
	}

	for {
		select {
		case <-stopCh:
			return
		case <-fallbackTicker.C:
			m.schedulerHeartbeat.Store(time.Now())
			if time.Since(lastUpdate) > 2*time.Second {
				weights = m.recalcWeights(weights, maxSchedulerWeight)
				lastUpdate = time.Now()
			}
			drainOnce()
		case <-m.schedulerSignal:
			m.schedulerHeartbeat.Store(time.Now())
			drainOnce()
		}
	}
}
