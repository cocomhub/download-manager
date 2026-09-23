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
	// 持锁写入 schedulerStop：Stop() 会 close 它，两者必须互斥（data race 保护）。
	// 注：resolveWorker/smallObjectWorker 均在 NewManager 构造时建立 ctx，
	// 与 Stop() 的 resolveCancel() 互斥由 resolveWg.Wait() 保证。
	m.mu.Lock()
	if m.schedulerEnabled.Load() {
		m.schedulerStop = make(chan struct{})
		go m.schedSvc.runScheduler()
	}
	m.mu.Unlock()

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

	// 启动标准化服务（异步，不阻塞启动）
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			select {
			case <-m.stopChan:
				cancel()
			case <-ctx.Done():
			}
		}()
		stdSvc := NewStandardizationService(m)
		stdSvc.Run(ctx)
		slog.Info("Initial standardization complete")
	}()

	// Immediate scan on start
	m.schedSvc.Scan()

	for {
		select {
		case <-ticker.C:
			m.schedSvc.Scan()
		case <-progressTicker.C:
			m.broadcastProgress()
		case <-m.stopChan:
			slog.Info("Manager stopping")
			m.mu.Lock()
			stop := m.schedulerStop
			if stop != nil {
				select {
				case <-stop:
					// already closed
				default:
					close(stop)
				}
			}
			m.mu.Unlock()
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

	// 2. 与 Start()/reconcileScheduler 的写入互斥：读 schedulerStop 前先取锁。
	m.mu.Lock()
	stop := m.schedulerStop
	if stop != nil {
		select {
		case <-stop:
			// already closed
		default:
			close(stop)
		}
	}
	m.mu.Unlock()

	// 2. Close idle connections on the transport
	if dl, ok := m.getDownloader().(interface{ CloseIdleConnections() }); ok {
		dl.CloseIdleConnections()
	}

	// 3. Wait for workers and force-downloads with context deadline
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
		// Clean up downloadingObj and activeDownloads — the defer in download()
		// may never run for items still in the queue when the worker picks
		// stopChan over the buffered channel.
		m.downloadingObj.Delete(key)
		m.mu.Lock()
		if obj != nil && m.activeDownloads[obj.TaskID] > 0 {
			m.activeDownloads[obj.TaskID]--
		}
		m.mu.Unlock()
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

// scan 兼容入口：委托 SchedulerService（历史调用方保持可用）。
// schedSvc 未初始化（&Manager{} 字面量测试）时直接返回。
func (m *Manager) scan() {
	if m.schedSvc == nil {
		return
	}
	m.schedSvc.Scan()
}

// processTask 兼容入口：委托 SchedulerService。
func (m *Manager) processTask(t core.Task) {
	if m.schedSvc == nil {
		return
	}
	m.schedSvc.processTask(t)
}

// scheduler 兼容入口：委托 SchedulerService（runScheduler）。
func (m *Manager) scheduler() {
	if m.schedSvc == nil {
		return
	}
	m.schedSvc.runScheduler()
}

// getTaskQueue 兼容入口：委托 SchedulerService。
// schedSvc 未初始化时回退到 Manager 自身的 taskQueues（保持原有容量计算语义，
// 供测试直接构造 &Manager{} 的场景）。
func (m *Manager) getTaskQueue(taskID string) chan *downloadRequest {
	if m.schedSvc == nil {
		return m.getTaskQueueSelf(taskID)
	}
	return m.schedSvc.getTaskQueue(taskID)
}

// getTaskQueueSelf 是原始实现（容量计算逻辑），供 schedSvc 未初始化时回退。
func (m *Manager) getTaskQueueSelf(taskID string) chan *downloadRequest {
	if v, ok := m.taskQueues.Load(taskID); ok {
		return v.(chan *downloadRequest)
	}
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
