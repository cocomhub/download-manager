// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/downloader"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

// isCancelled 从 storage 重新读取对象状态检查是否已取消，
// 避免 download() 中的本地指针（obj）可能在 CancelObject 修改 storage 后变为 stale。
func (m *Manager) isCancelled(t core.Task, obj *model.DownloadObject) bool {
	if obj.GetStatus() == model.StatusCancelled {
		return true
	}
	st := t.Storage()
	if st == nil {
		return false
	}
	current, err := st.Get(obj.URL)
	if err != nil || current == nil {
		return false
	}
	return current.GetStatus() == model.StatusCancelled
}

func (m *Manager) download(t core.Task, obj *model.DownloadObject) {
	start := time.Now()
	defer func() {
		// Only decrement activeDownloads if the object is still tracked in downloadingObj.
		// CancelTask/CancelObject may have already handled cleanup, including the
		// decrement and downloadingObj.Delete. Without this check, the double-decrement
		// would make activeDownloads negative.
		if _, stillActive := m.downloadingObj.Load(obj.URL); stillActive {
			m.mu.Lock()
			m.activeDownloads[t.ID()]--
			m.mu.Unlock()
		}

		// Remove from downloadingObj map (safe to call even if CancelTask already removed it)
		m.downloadingObj.Delete(obj.URL)
		m.lastProgress.Delete(obj.URL)

		// Broadcast task update on finish
		m.BroadcastTaskUpdate(t.ID())

		// 通知调度器：可能有新槽位可用
		select {
		case m.schedulerSignal <- struct{}{}:
		default:
		}
	}()

	// Check if manager is stopping — avoids overwriting status set by Stop()
	select {
	case <-m.stopChan:
		slog.Info("Download skipped — manager stopping", logutil.LogKeyURL, obj.URL)
		return
	default:
	}

	// 定期清理小对象 tracker，防止内存泄漏
	defer m.soTracker.Delete(obj.URL)

	// 检查 resolve 是否过期，过期则重新 resolve
	if m.resolveCache.IsExpired(obj.URL) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := t.ResolveObject(ctx, obj); err != nil {
			slog.Error("Download: resolve expired and re-resolve failed", logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
			t.UpdateStatus(obj, model.StatusFailed, err)
			return
		}
		m.resolveCache.MarkResolved(obj.URL)
		slog.Debug("Download: re-resolved expired object", logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL)
	}

	// 发起小对象下载（不阻塞主体下载）
	m.enqueueSmallObjects(t, obj)

	t.UpdateStatus(obj, model.StatusDownloading, nil)
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
	m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})

	dl := m.getDownloader()

	// Create per-download context tied to manager lifecycle for cancellation
	dlCtx, dlCancel := context.WithCancel(context.Background())
	defer dlCancel()
	go func() {
		select {
		case <-m.stopChan:
			dlCancel()
		case <-dlCtx.Done():
		}
	}()

	// Periodic worker heartbeat during long downloads
	heartbeatStop := make(chan struct{})
	defer close(heartbeatStop)
	go func() {
		hbTick := time.NewTicker(5 * time.Second)
		defer hbTick.Stop()
		for {
			select {
			case <-hbTick.C:
				m.workerHeartbeat.Store(time.Now())
			case <-heartbeatStop:
				return
			case <-dlCtx.Done():
				return
			}
		}
	}()

	// Propagate context to downloader if supported
	if nd, ok := dl.(core.DownloaderWithContext); ok {
		nd.SetContext(dlCtx)
	}

	// 设置 metadata flusher：在 Extractor 提取到 ETag/checksum 后立即持久化到存储，
	// 避免进程崩溃时丢失元数据导致下次必须重新下载。
	if mf, ok := dl.(interface{ SetMetadataFlusher(func()) }); ok {
		mf.SetMetadataFlusher(func() {
			st := t.Storage()
			if st == nil {
				return
			}
			if err := st.Update(obj); err != nil {
				slog.Error("Metadata flush: store.Update failed",
					logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
				return
			}
			if flusher, ok := st.(interface{ ForceFlush() error }); ok {
				if err := flusher.ForceFlush(); err != nil {
					slog.Error("Metadata flush: ForceFlush failed",
						logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
				}
			}
		})
	} else {
		slog.Warn("Metadata flush not supported — crash may lose ETag/checksum",
			logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL)
	}

	err := dl.Download(obj, t.GetDownloadHeaders())
	if err != nil {
		if m.isCancelled(t, obj) {
			m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
			m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
			return
		}

		// 复合下载空列表：重新 resolve 后放回 pending，让调度器重新调度
		// 最多 10 次，指数退避最大 1h
		if errors.Is(err, downloader.ErrCompositeEmpty) {
			v, _ := m.compositeResolveCount.LoadOrStore(obj.URL, new(atomic.Int64))
			counter, ok := v.(*atomic.Int64)
			if !ok {
				m.compositeResolveCount.Store(obj.URL, new(atomic.Int64))
				raw, _ := m.compositeResolveCount.Load(obj.URL)
				counter, ok = raw.(*atomic.Int64)
				if !ok {
					m.compositeResolveCount.Store(obj.URL, new(atomic.Int64))
					counter = new(atomic.Int64)
					m.compositeResolveCount.Store(obj.URL, counter)
				}
			}
			count := counter.Add(1)
			if count > 10 {
				slog.Error("Composite resolve retry exhausted, marking as permanent failure",
					logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL)
				m.compositeResolveCount.Delete(obj.URL)
				t.UpdateStatus(obj, model.StatusFailedPermanent, err)
				if ft, ok := t.(core.FailedTask); ok {
					ft.MarkAsFailed(obj, err)
				}
				m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
				m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
				return
			}

			// 指数退避：2^(count-1) 秒，最大 3600 秒
			backoff := min(time.Duration(1<<(count-1))*time.Second, time.Hour)
			slog.Warn("Composite download with empty file list, re-resolving",
				logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL, "attempt", count, "backoff", backoff)

			time.Sleep(backoff)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if resolveErr := t.ResolveObject(ctx, obj); resolveErr != nil {
				slog.Error("Re-resolve after composite empty failed",
					logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL, logutil.LogKeyError, resolveErr)
			}
			t.UpdateStatus(obj, model.StatusPending, nil)
			m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
			m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
			select {
			case m.schedulerSignal <- struct{}{}:
			default:
			}
			return
		}

		slog.Error("Download failed", logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
		t.UpdateStatus(obj, model.StatusFailed, err)
		mt := m.getOrCreateMetrics(t.ID())
		mt.failures.Add(1)
		mt.lastActive.Store(time.Now().Unix())

		if download.IsNoTry(err) {
			if ft, ok := t.(core.FailedTask); ok {
				ft.MarkAsFailed(obj, err)
			}
		}

		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return
		}

		// Increment failed count
		v, _ := m.failedCount.LoadOrStore(obj.URL, new(atomic.Int64))
		counter, ok := v.(*atomic.Int64)
		if !ok {
			m.failedCount.Store(obj.URL, new(atomic.Int64))
			raw, _ := m.failedCount.Load(obj.URL)
			counter, ok = raw.(*atomic.Int64)
			if !ok {
				fallback := new(atomic.Int64)
				m.failedCount.Store(obj.URL, fallback)
				counter = fallback
			}
		}
		c := counter.Add(1)
		// Track retries (c > 1 means this is a retry)
		if c > 1 {
			m.getOrCreateMetrics(t.ID()).retried.Add(1)
		}
		// Check if max retries reached
		maxRetries := m.currentCfg().Downloader.MaxRetries
		if maxRetries <= 0 {
			maxRetries = 5
		}
		if c >= int64(maxRetries) {
			if ft, ok := t.(core.FailedTask); ok {
				ft.MarkAsFailed(obj, fmt.Errorf("max retries reached: %w", err))
			}
		}

		// Record failure detail
		isPermanent := download.IsNoTry(err) || (maxRetries > 0 && c >= int64(maxRetries))
		m.recordFailure(t.ID(), obj.URL, err.Error(), int(c), isPermanent)
	} else {
		// 等待小对象完成后再标记 Completed
		if soTracker := m.soTrackerForObj(obj.URL); soTracker != nil {
			errs := soTracker.WaitAll(5 * time.Minute)
			for _, e := range errs {
				if e != nil {
					slog.Warn("Small-object download had error", logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL, logutil.LogKeyError, e)
				}
			}
			m.soTracker.Delete(obj.URL)
		}

		// 检查是否已被取消（CancelObject 可能已在别的 goroutine 中修改了 storage）
		if m.isCancelled(t, obj) {
			slog.Info("Download: object was cancelled before completion, preserving cancelled status",
				logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL)
			m.failedCount.Delete(obj.URL)
			return
		}

		t.UpdateStatus(obj, model.StatusCompleted, nil)
		// Reset failed count on success
		m.failedCount.Delete(obj.URL)
		m.totalDownloads.Add(1)
		// Apply group priority policies for content groups
		m.applyGroupPriorityPolicies(t, obj)
		mt := m.getOrCreateMetrics(t.ID())
		mt.completed.Add(1)
		elapsed := time.Since(start).Seconds() * 1000
		if mt.avgLatencyMs.Load() == 0 {
			mt.avgLatencyMs.Store(int64(elapsed))
		} else {
			for {
				old := mt.avgLatencyMs.Load()
				newVal := int64(float64(old)*0.7 + elapsed*0.3)
				if mt.avgLatencyMs.CompareAndSwap(old, newVal) {
					break
				}
			}
		}
		mt.lastActive.Store(time.Now().Unix())
	}
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
	m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
}

// soTrackerForObj 返回指定对象的小对象 tracker（如果存在）。
func (m *Manager) soTrackerForObj(url string) *objectTracker {
	if v, ok := m.soTracker.Load(url); ok {
		return v.(*objectTracker)
	}
	return nil
}

// forceDownload bypasses the queue and runs immediately
func (m *Manager) forceDownload(t core.Task, obj *model.DownloadObject) {
	if _, loaded := m.downloadingObj.LoadOrStore(obj.URL, obj); loaded {
		return // Already downloading
	}

	slog.Info("Force starting download", logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL)

	m.mu.Lock()
	m.activeDownloads[t.ID()]++
	m.mu.Unlock()

	// Run in separate goroutine, bypassing worker pool limits
	m.forceWg.Go(func() {
		m.download(t, obj)
	})
}

// getOrCreateMetrics 返回 taskID 对应的 taskMetrics，不存在时新建。
func (m *Manager) getOrCreateMetrics(taskID string) *taskMetrics {
	if v, ok := m.metrics.Load(taskID); ok {
		return v.(*taskMetrics)
	}
	mt := &taskMetrics{}
	if v, loaded := m.metrics.LoadOrStore(taskID, mt); loaded {
		return v.(*taskMetrics)
	}
	return mt
}

// RetryObject resets the status of an object to pending and forces download
func (m *Manager) RetryObject(taskID, url string) error {
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

// RetryAllFailed resets all failed objects in a task
func (m *Manager) RetryAllFailed(taskID string) error {
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
