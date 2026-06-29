// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
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
	defer m.cleanupAfterDownload(t, obj)

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
			if obj.GetStatus() != model.StatusFailedPermanent {
				t.UpdateStatus(obj, model.StatusFailed, err)
			}
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
	go m.heartbeatLoop(heartbeatStop, dlCtx)

	// Propagate context to downloader if supported
	if nd, ok := dl.(core.ContextInjecter); ok {
		nd.SetContext(dlCtx)
	}

	// 设置 metadata flusher
	m.setupMetadataFlusher(dl, t, obj)

	err := dl.Download(obj, t.GetDownloadHeaders())
	if err != nil {
		m.handleDownloadError(t, obj, err)
	} else {
		m.handleDownloadSuccess(t, obj, start)
	}
}

// cleanupAfterDownload 处理下载结束后的清理工作：释放 activeDownloads、删除 downloadingObj 跟踪、广播任务更新、通知调度器。
func (m *Manager) cleanupAfterDownload(t core.Task, obj *model.DownloadObject) {
	if _, stillActive := m.downloadingObj.Load(obj.URL); stillActive {
		m.mu.Lock()
		m.activeDownloads[t.ID()]--
		m.mu.Unlock()
	}

	m.downloadingObj.Delete(obj.URL)
	m.lastProgress.Delete(obj.URL)

	m.BroadcastTaskUpdate(t.ID())

	select {
	case m.schedulerSignal <- struct{}{}:
	default:
	}
}

// heartbeatLoop 在下载期间定期更新 worker heartbeat，防止 health check 超时。
func (m *Manager) heartbeatLoop(stop <-chan struct{}, ctx context.Context) {
	hbTick := time.NewTicker(5 * time.Second)
	defer hbTick.Stop()
	for {
		select {
		case <-hbTick.C:
			m.workerHeartbeat.Store(time.Now())
		case <-stop:
			return
		case <-ctx.Done():
			return
		}
	}
}

// setupMetadataFlusher 注册 metadata flusher 回调，确保 ETag/checksum 在提取后立即持久化。
func (m *Manager) setupMetadataFlusher(dl core.Downloader, t core.Task, obj *model.DownloadObject) {
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
}

// handleDownloadError 根据错误类型分发处理：取消、复合空列表、或一般下载错误。
func (m *Manager) handleDownloadError(t core.Task, obj *model.DownloadObject, err error) {
	if m.isCancelled(t, obj) {
		m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
		m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
		return
	}
	if errors.Is(err, downloader.ErrCompositeEmpty) {
		m.handleCompositeEmptyError(t, obj, err)
		return
	}
	m.handleGeneralDownloadError(t, obj, err)
}

// handleCompositeEmptyError 处理复合下载空列表错误：重新 resolve 后放回 pending，指数退避。
func (m *Manager) handleCompositeEmptyError(t core.Task, obj *model.DownloadObject, err error) {
	counter := syncMapCounter(&m.compositeResolveCount, obj.URL)
	count := counter.Add(1)
	if count > 10 {
		slog.Error("Composite resolve retry exhausted, marking as permanent failure",
			logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL)
		m.compositeResolveCount.Delete(obj.URL)
		t.UpdateStatus(obj, model.StatusFailedPermanent, err)
		if ft, ok := t.(core.FailedTaskMarker); ok {
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
}

// handleGeneralDownloadError 处理一般下载失败（非 composite empty）。
func (m *Manager) handleGeneralDownloadError(t core.Task, obj *model.DownloadObject, err error) {
	slog.Error("Download failed", logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
	t.UpdateStatus(obj, model.StatusFailed, err)
	mt := m.getOrCreateMetrics(t.ID())
	mt.failures.Add(1)
	mt.lastActive.Store(time.Now().Unix())

	if download.IsNoTry(err) {
		if ft, ok := t.(core.FailedTaskMarker); ok {
			ft.MarkAsFailed(obj, err)
		}
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return
	}

	counter := syncMapCounter(&m.failedCount, obj.URL)
	c := counter.Add(1)
	if c > 1 {
		m.getOrCreateMetrics(t.ID()).retried.Add(1)
	}

	maxRetries := m.currentCfg().Downloader.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 5
	}
	if c >= int64(maxRetries) {
		if ft, ok := t.(core.FailedTaskMarker); ok {
			ft.MarkAsFailed(obj, fmt.Errorf("max retries reached: %w", err))
		}
	}

	isPermanent := download.IsNoTry(err) || (maxRetries > 0 && c >= int64(maxRetries))
	m.recordFailure(t.ID(), obj.URL, err.Error(), int(c), isPermanent)
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
	m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
}

// handleDownloadSuccess 处理下载成功后的逻辑：等待小对象、检查取消、标记 completed、更新 metrics。
func (m *Manager) handleDownloadSuccess(t core.Task, obj *model.DownloadObject, start time.Time) {
	if soTracker := m.soTrackerForObj(obj.URL); soTracker != nil {
		errs := soTracker.WaitAll(5 * time.Minute)
		for _, e := range errs {
			if e != nil {
				slog.Warn("Small-object download had error", logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL, logutil.LogKeyError, e)
			}
		}
		m.soTracker.Delete(obj.URL)
	}

	if m.isCancelled(t, obj) {
		slog.Info("Download: object was cancelled before completion, preserving cancelled status",
			logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, obj.URL)
		m.failedCount.Delete(obj.URL)
		return
	}

	t.UpdateStatus(obj, model.StatusCompleted, nil)
	m.failedCount.Delete(obj.URL)
	m.totalDownloads.Add(1)
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
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
	m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
}

// syncMapCounter 从 sync.Map 加载或存储 atomic.Int64 计数器，应对类型断言竞争，返回稳定的 *atomic.Int64。
func syncMapCounter(m *sync.Map, key string) *atomic.Int64 {
	v, _ := m.LoadOrStore(key, new(atomic.Int64))
	counter, ok := v.(*atomic.Int64)
	if !ok {
		m.Store(key, new(atomic.Int64))
		raw, _ := m.Load(key)
		counter, ok = raw.(*atomic.Int64)
		if !ok {
			fallback := new(atomic.Int64)
			m.Store(key, fallback)
			counter = fallback
		}
	}
	return counter
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
