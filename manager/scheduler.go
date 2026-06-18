// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

func (m *Manager) Start() {
	slog.Info("Manager started")
	cfg := m.currentCfg()
	slog.Info("runtime mode", "mode", cfg.Runtime.Mode, "download", cfg.Runtime.Download.Enabled, "scheduler", cfg.Runtime.Scheduler.Enabled)
	m.workersEnabled.Store(cfg.Runtime.Mode != config.RunModeUI && cfg.Runtime.Download.Enabled)
	m.schedulerEnabled.Store(cfg.Runtime.Mode != config.RunModeUI && cfg.Runtime.Scheduler.Enabled)
	slog.Info("disabled components", "scheduler", !m.schedulerEnabled.Load(), "workers", !m.workersEnabled.Load())
	m.loadTasks()
	if m.workersEnabled.Load() {
		limit := m.currentCfg().Downloader.GlobalConcurrent
		if limit <= 0 {
			limit = 5
		}
		slog.Info("Starting global workers", "count", limit)
		m.mu.Lock()
		for i := 0; i < limit; i++ {
			m.workerWg.Go(m.worker)
		}
		m.workerCount.Store(int64(limit))
		m.mu.Unlock()
	}
	if m.workersEnabled.Load() {
		m.StartResolveWorkers(3)
		m.StartSmallObjectWorkers(2)
	}
	if m.schedulerEnabled.Load() {
		m.schedulerStop = make(chan struct{})
		go m.scheduler()
	}

	interval := time.Duration(m.currentCfg().TaskScan.Interval) * time.Second
	if interval == 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)

	// Progress broadcast ticker
	progressTicker := time.NewTicker(1 * time.Second)

	defer ticker.Stop()
	defer progressTicker.Stop()

	// Signal that initialization is complete (guards against race with UpdateConfig).
	// Must happen before the infinite for-loop below since defer would never fire.
	close(m.initializedCh)

	// Immediate scan on start
	m.scan()

	for {
		select {
		case <-ticker.C:
			m.scan()
		case <-progressTicker.C:
			m.broadcastProgress()
		case <-m.stopChan:
			slog.Info("Manager stopping")
			if m.schedulerStop != nil {
				select {
				case <-m.schedulerStop:
					// already closed
				default:
					close(m.schedulerStop)
				}
			}
			m.closeAllTasks()
			return
		}
	}
}

func (m *Manager) Stop(ctx context.Context) {
	slog.Info("Manager stopping")

	// 1. Signal workers to stop first — no new downloads
	close(m.stopChan)
	m.StopResolveWorkers()
	m.StopSmallObjectWorkers()

	// 2. Wait for workers and force-downloads with context deadline
	done := make(chan struct{})
	go func() {
		m.workerWg.Wait()
		m.forceWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("All workers stopped")
	case <-ctx.Done():
		slog.Warn("Shutdown timed out, some workers may still be running")
	}

	// 3. Mark survivors (e.g. force-download goroutines that didn't finish) as failed
	m.downloadingObj.Range(func(key, value any) bool {
		obj := value.(*model.DownloadObject)
		if t, ok := m.getTask(obj.TaskID); ok {
			t.UpdateStatus(obj, model.StatusFailed, errors.New("shutdown"))
			m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
			m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
		}
		return true
	})
}

// WaitForShutdown waits for workers and force-downloads to finish, then flushes storages.
// It respects the provided context deadline.
func (m *Manager) WaitForShutdown(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		m.workerWg.Wait()
		m.forceWg.Wait()
		m.flushAllStorages()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("All workers stopped and storages flushed")
	case <-ctx.Done():
		slog.Warn("Shutdown timed out, some workers may still be running")
	}
}

func (m *Manager) scan() {
	// slog.Debug("Scanning tasks")
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
		if sc, ok := value.(core.ScrapeCap); ok {
			taskID := key.(string)
			if _, scraping := m.scrapingTask.LoadOrStore(taskID, true); scraping {
				slog.Debug("Scrape: previous run still in progress, skipping", "task_id", taskID)
				return true
			}
			go func(taskID string, sc core.ScrapeCap) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				done := make(chan error, 1)
				go func() {
					done <- sc.Scrape(ctx)
				}()
				select {
				case err := <-done:
					if err != nil {
						slog.Error("Scrape failed", "task_id", taskID, "error", err)
					}
				case <-ctx.Done():
					slog.Error("Scrape timed out", "task_id", taskID)
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

		go m.processTask(t)
	}
}

func (m *Manager) processTask(t core.Task) {
	defer m.processingTask.Delete(t.ID())

	// Check per-task concurrency limit (soft limit for scheduling?)
	// If global limit is used, task limit might be redundant or acts as "fairness" limit.
	// Let's keep it.

	limit := t.Concurrency()

	m.mu.Lock()
	active := m.activeDownloads[t.ID()]
	// If active >= limit, we stop scheduling new downloads for this task.
	if active >= limit {
		m.mu.Unlock()
		// slog.Debug("Task reached concurrency limit", "task_id", t.ID(), "active", active, "limit", limit)
		return
	}
	m.mu.Unlock()

	// Calculate remaining slots
	slotsAvailable := max(0, limit-active)

	// Only fetch objects if we have capacity
	objs, err := t.GetDownloadObjects()
	if err != nil {
		slog.Error("Error getting objects for task", "task_id", t.ID(), "error", err)
		return
	}

	if len(objs) == 0 {
		return
	}
	// slog.Debug("Task has objects to download", "task_id", t.ID(), "count", len(objs))

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
			slog.Info("Object enqueued", "task_id", t.ID(), "url", obj.URL)

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

func (m *Manager) getTaskQueue(taskID string) chan *downloadRequest {
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

func (m *Manager) scheduler() {
	fallbackTicker := time.NewTicker(500 * time.Millisecond)
	defer fallbackTicker.Stop()
	weights := make(map[string]int)
	lastUpdate := time.Now()

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
		case <-m.schedulerStop:
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
