// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"time"
)

// CollectMetrics 收集当前所有任务和全局的运行时指标
func (m *Manager) CollectMetrics() map[string]any {
	taskMetricsMap := make(map[string]any)

	m.metrics.Range(func(key, value any) bool {
		id := key.(string)
		mt := value.(*taskMetrics)

		// 获取活跃下载数
		m.mu.Lock()
		active := m.activeDownloads[id]
		m.mu.Unlock()

		// 获取队列深度
		q := m.getTaskQueue(id)

		// 获取并发度
		concurrency := 2 // default
		if t, ok := m.getTask(id); ok {
			concurrency = t.Concurrency()
		}

		taskMetricsMap[id] = map[string]any{
			"avg_latency_ms": mt.avgLatencyMs.Load(),
			"completed":      mt.completed.Load(),
			"failures":       mt.failures.Load(),
			"retried":        mt.retried.Load(),
			"last_active":    mt.lastActive.Load(),
			"queue_depth":    len(q),
			"active":         active,
			"concurrency":    concurrency,
		}
		return true
	})

	// Global metrics
	activeDownloads := 0
	m.downloadingObj.Range(func(_, _ any) bool {
		activeDownloads++
		return true
	})

	schedulerStatus := "stopped"
	if m.schedulerEnabled.Load() {
		schedulerStatus = "running"
	}

	m.eventMu.RLock()
	subCount := len(m.subscribers)
	m.eventMu.RUnlock()

	global := map[string]any{
		"active_downloads": activeDownloads,
		"worker_count":     m.workerCount.Load(),
		"scheduler":        schedulerStatus,
		"total_downloads":  m.totalDownloads.Load(),
		"subscriber_count": subCount,
	}

	return map[string]any{
		"uptime": time.Since(m.startedAt).Round(time.Second).String(),
		"tasks":  taskMetricsMap,
		"global": global,
	}
}

// recordFailure 记录一条失败记录到环形缓冲区
func (m *Manager) recordFailure(taskID, url, errStr string, attempt int, permanent bool) {
	m.failureMu.Lock()
	defer m.failureMu.Unlock()

	idx := m.failureWriteIdx % m.maxFailures
	m.failureRecords[idx] = FailureRecord{
		TaskID:    taskID,
		URL:       url,
		Error:     errStr,
		Attempt:   attempt,
		Timestamp: time.Now().Unix(),
		Permanent: permanent,
	}
	m.failureWriteIdx++
}

// GetFailures 查询失败记录列表
func (m *Manager) GetFailures(taskID string, limit int) map[string]any {
	if limit <= 0 || limit > 200 {
		limit = 200
	}

	m.failureMu.Lock()
	total := m.failureWriteIdx
	count := min(m.failureWriteIdx, m.maxFailures)
	records := make([]FailureRecord, 0, count)
	for i := range count {
		idx := (m.failureWriteIdx - 1 - i + m.maxFailures) % m.maxFailures
		r := m.failureRecords[idx]
		if r.TaskID == "" {
			continue
		}
		if taskID != "" && r.TaskID != taskID {
			continue
		}
		records = append(records, r)
		if len(records) >= limit {
			break
		}
	}
	m.failureMu.Unlock()

	if records == nil {
		records = make([]FailureRecord, 0)
	}
	return map[string]any{
		"failures": records,
		"total":    total,
	}
}
