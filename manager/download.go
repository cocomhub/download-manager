// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/downloader"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"
	"github.com/cocomhub/download-manager/task/tktube"
)

func (m *Manager) download(t core.Task, obj *model.DownloadObject) {
	start := time.Now()
	defer func() {
		m.mu.Lock()
		m.activeDownloads[t.ID()]--
		m.mu.Unlock()

		// Remove from downloadingObj map
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
		slog.Info("Download skipped — manager stopping", "url", obj.URL)
		return
	default:
	}

	t.UpdateStatus(obj, dlcore.StatusDownloading, nil)
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
	m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})

	m.mu.Lock()
	dl := m.downloader
	m.mu.Unlock()

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

	// Propagate context to NativeHTTPDownloader if supported
	if nd, ok := dl.(*downloader.NativeHTTPDownloader); ok {
		nd.SetContext(dlCtx)
	}

	err := dl.Download(obj, t.GetDownloadHeaders())
	if err != nil {
		if obj.GetStatus() == dlcore.StatusCancelled {
			m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
			m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
			return
		}
		slog.Error("Download failed", "task_id", t.ID(), "url", obj.URL, "error", err)
		t.UpdateStatus(obj, dlcore.StatusFailed, err)
		if v, ok := m.metrics.LoadOrStore(t.ID(), &taskMetrics{}); ok {
			v.(*taskMetrics).lastActive.Store(time.Now().Unix())
		}

		if dlcore.IsNoTry(err) {
			if ft, ok := t.(core.FailedTask); ok {
				ft.MarkAsFailed(obj, err)
			}
		}

		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return
		}

		// Increment failed count
		v, _ := m.failedCount.LoadOrStore(obj.URL, new(atomic.Int64))
		c := v.(*atomic.Int64).Add(1)
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
	} else {
		t.UpdateStatus(obj, dlcore.StatusCompleted, nil)
		// Reset failed count on success
		m.failedCount.Delete(obj.URL)
		m.totalDownloads.Add(1)
		// Apply group priority policies for content groups
		m.applyGroupPriorityPolicies(t, obj)
		if v, ok := m.metrics.LoadOrStore(t.ID(), &taskMetrics{}); ok {
			mt := v.(*taskMetrics)
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
		}
		if v, ok := m.metrics.LoadOrStore(t.ID(), &taskMetrics{}); ok {
			v.(*taskMetrics).lastActive.Store(time.Now().Unix())
		}
	}
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
	m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
}

// forceDownload bypasses the queue and runs immediately
func (m *Manager) forceDownload(t core.Task, obj *model.DownloadObject) {
	if _, loaded := m.downloadingObj.LoadOrStore(obj.URL, obj); loaded {
		return // Already downloading
	}

	slog.Info("Force starting download", "task_id", t.ID(), "url", obj.URL)

	m.mu.Lock()
	m.activeDownloads[t.ID()]++
	m.mu.Unlock()

	// Run in separate goroutine, bypassing worker pool limits
	m.forceWg.Go(func() {
		m.download(t, obj)
	})
}

// applyGroupPriorityPolicies enforces group priority within the current tktube task only.
// Even if multiple tasks share the same storage, only objects whose TaskID matches t.ID()
// and whose task_type/content_group match the completed object are eligible.
func (m *Manager) applyGroupPriorityPolicies(t core.Task, obj *model.DownloadObject) {
	if t.Type() != tktube.TaskType {
		return
	}
	if obj == nil || obj.GetStatus() != dlcore.StatusCompleted {
		return
	}
	taskType := strings.TrimSpace(t.Type())
	if taskType == "" || metadataTaskType(obj) != taskType {
		return
	}
	group := metadataContentGroup(obj)
	if strings.TrimSpace(group) == "" {
		return
	}
	taskID := strings.TrimSpace(t.ID())
	if taskID == "" || strings.TrimSpace(obj.TaskID) != taskID {
		return
	}
	st := t.Storage()
	if st == nil {
		return
	}
	list, err := m.collectTaskObjects(t, &core.StorageQuery{
		Filter: core.StorageFilter{
			Metadata: map[string]string{"task_type": taskType, "content_group": group},
		},
	}, 200)
	if err != nil || list == nil {
		return
	}
	type candidate struct {
		o     *model.DownloadObject
		score int
	}
	var canonical *model.DownloadObject
	bestScore := -1
	cands := make([]candidate, 0, 8)
	priorityCounts := make(map[int]int, 4)
	for _, o := range list {
		if o == nil {
			continue
		}
		if strings.TrimSpace(o.TaskID) != taskID {
			continue
		}
		if metadataTaskType(o) != taskType {
			continue
		}
		if metadataContentGroup(o) != group {
			continue
		}
		score := variantPriorityScore(t, o)
		cands = append(cands, candidate{o: o, score: score})
		priorityCounts[score]++
		if o.GetStatus() == dlcore.StatusCompleted {
			if canonical == nil || score > bestScore {
				canonical = o
				bestScore = score
			}
		}
	}
	for priority, count := range priorityCounts {
		if count > 1 {
			slog.Info("Skip auto-cancel for conflicting content group priority", "task_id", t.ID(), "task_type", t.Type(), "content_group", group, "priority", priority, "count", count)
			return
		}
	}
	if canonical == nil {
		return
	}
	for _, cnd := range cands {
		o := cnd.o
		if o.URL == canonical.URL {
			continue
		}
		// Auto-cancel only lower-priority pending objects.
		if cnd.score < bestScore && o.GetStatus() == dlcore.StatusPending {
			if o.Extra == nil {
				o.Extra = make(map[string]any)
			}
			o.Extra["redirect_url"] = canonical.URL
			if err := t.UpdateStatus(o, dlcore.StatusCancelled, nil); err != nil {
				slog.Warn("Failed to auto-cancel lower-priority duplicate", "task_id", t.ID(), "url", o.URL, "error", err)
			}
		}
	}
}

// RetryObject resets the status of an object to pending and forces download
func (m *Manager) RetryObject(taskID, url string) error {
	t, ok := m.getTask(taskID)

	if !ok {
		return fmt.Errorf("task not found")
	}

	obj, err := m.getTaskObject(t, url)
	if err != nil {
		return err
	}
	if obj != nil {
		if obj.GetStatus() == dlcore.StatusCompleted {
			return fmt.Errorf("object already completed")
		}
		// Reset status
		t.UpdateStatus(obj, dlcore.StatusPending, nil)
		obj.SetProgress(0)

		// Resolve details if needed (JIT for forced retry?)
		if resolver, ok := t.(interface {
			ResolveObject(*model.DownloadObject) error
		}); ok {
			slog.Info("Resolving object before retry", "task_id", taskID, "url", url)
			if err := resolver.ResolveObject(obj); err != nil {
				slog.Error("Failed to resolve object for retry", "error", err)
				return fmt.Errorf("failed to resolve object: %v", err)
			}
		}

		m.forceDownload(t, obj)
		if v, ok := m.metrics.LoadOrStore(t.ID(), &taskMetrics{}); ok {
			v.(*taskMetrics).retried.Add(1)
		}
		return nil
	}
	return fmt.Errorf("object not found")
}

// RetryAllFailed resets all failed objects in a task
func (m *Manager) RetryAllFailed(taskID string) error {
	t, ok := m.getTask(taskID)

	if !ok {
		return fmt.Errorf("task not found")
	}

	objs, err := m.collectTaskObjects(t, &core.StorageQuery{
		Filter: core.StorageFilter{
			Statuses: []string{dlcore.StatusFailed, dlcore.StatusFailedPermanent},
		},
	}, 200)
	if err != nil {
		return err
	}
	count := 0
	for _, obj := range objs {
		t.UpdateStatus(obj, dlcore.StatusPending, nil)
		obj.SetProgress(0)
		if v, ok := m.metrics.LoadOrStore(t.ID(), &taskMetrics{}); ok {
			v.(*taskMetrics).retried.Add(1)
		}
		count++
	}
	if count > 0 {
		go m.processTask(t)
		// 通知调度器：有新的待处理对象
		select {
		case m.schedulerSignal <- struct{}{}:
		default:
		}
	}
	return nil
}
