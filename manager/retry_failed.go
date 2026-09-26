// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"log/slog"
	"sort"
	"sync/atomic"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

// retryFailedPermanent 限流自动重试 failed_permanent 对象。
// 策略：每小时选「失败次数最少」的一批重试（避免再次触发源站拦截，如 CF 限速恢复后自然可下）。
// - 失败次数来自 m.failedCount（URL → 计数）；重启后丢失 → 归 0（同权重，轮转选批）
// - 每批数量 = cfg.Downloader.Retry.BatchSize（可配置）
// - 选中对象置回 pending + 通知调度器（不 bypass 队列，走正常调度）
func (m *Manager) retryFailedPermanent() int {
	rc := m.currentCfg().Downloader.Retry
	if !rc.Enabled {
		return 0
	}
	batchSize := rc.BatchSize
	if batchSize <= 0 {
		batchSize = 10
	}

	type candidate struct {
		task  core.Task
		obj   *model.DownloadObject
		fails int64
	}
	var cands []candidate

	m.tasks.Range(func(key, value any) bool {
		t := value.(core.Task)
		objs, err := m.collectTaskObjects(t, &core.StorageQuery{
			Filter: core.StorageFilter{
				Statuses: []string{model.StatusFailedPermanent},
			},
		}, 200)
		if err != nil {
			slog.Debug("retryFailedPermanent: collect failed", logutil.LogKeyTaskID, t.ID(), logutil.LogKeyError, err)
			return true
		}
		for _, obj := range objs {
			fails := m.failureCountFor(obj.URL)
			cands = append(cands, candidate{task: t, obj: obj, fails: fails})
		}
		return true
	})

	if len(cands) == 0 {
		return 0
	}

	// 按失败次数升序（最少失败优先重试 = 最可能成功），稳定排序保持轮转公平
	sort.SliceStable(cands, func(i, j int) bool {
		return cands[i].fails < cands[j].fails
	})

	if len(cands) > batchSize {
		cands = cands[:batchSize]
	}

	retried := 0
	for _, c := range cands {
		cur := c.obj.GetStatus()
		if cur != model.StatusFailedPermanent {
			continue // 已被人工重试/完成
		}
		// 置回 pending，走正常调度（不 bypass 队列——尊重并发限制）
		c.task.UpdateStatus(c.obj, model.StatusPending, nil)
		c.obj.SetProgress(0)
		retried++
	}
	if retried > 0 {
		slog.Info("Retry failed_permanent batch", "count", retried, "batch_size", batchSize)
		select {
		case m.schedulerSignal <- struct{}{}:
		default:
		}
	}
	return retried
}

// failureCountFor 返回 URL 的失败计数（failedCount sync.Map，成功时 Delete）。
func (m *Manager) failureCountFor(url string) int64 {
	v, ok := m.failedCount.Load(url)
	if !ok {
		return 0
	}
	if c, ok := v.(*atomic.Int64); ok {
		return c.Load()
	}
	return 0
}

// runRetryTicker 按 cfg interval 触发 retryFailedPermanent（每轮检查 enabled）。
func (m *Manager) runRetryTicker(stop <-chan struct{}) {
	interval := time.Duration(m.currentCfg().Downloader.Retry.IntervalHours) * time.Hour
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if n := m.retryFailedPermanent(); n > 0 {
				slog.Info("Auto retry ticker round done", "retried", n)
			}
		case <-stop:
			return
		}
	}
}
