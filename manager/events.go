// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"log/slog"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// ProgressBatch 鍖呭惈涓€娆″箍鎾懆鏈熷唴鎵€鏈夊璞＄殑杩涘害鍙樻洿
type ProgressBatch struct {
	Updates []ProgressItem `json:"updates"`
}

type ProgressItem struct {
	TaskID   string `json:"task_id"`
	URL      string `json:"url"`
	Progress int    `json:"progress"`
	Status   string `json:"status"`
	Title    string `json:"title,omitempty"`
}

func (m *Manager) Subscribe() <-chan core.Event {
	m.eventMu.Lock()
	defer m.eventMu.Unlock()
	ch := make(chan core.Event, 100) // Buffer to prevent blocking
	m.subscribers[ch] = ch
	return ch
}

func (m *Manager) Unsubscribe(ch <-chan core.Event) {
	m.eventMu.Lock()
	defer m.eventMu.Unlock()
	if c, ok := m.subscribers[ch]; ok {
		close(c)
		delete(m.subscribers, ch)
	}
}

func (m *Manager) publish(e core.Event) {
	m.eventMu.RLock()
	defer m.eventMu.RUnlock()
	for _, ch := range m.subscribers {
		select {
		case ch <- e:
		default:
			// Drop event if consumer is too slow
			slog.Warn("Dropping event for slow subscriber", "type", e.Type)
		}
	}
}

func (m *Manager) broadcastProgress() {
	batch := &ProgressBatch{
		Updates: make([]ProgressItem, 0, 64),
	}
	m.downloadingObj.Range(func(key, value any) bool {
		obj := value.(*model.DownloadObject)

		// Check if progress has changed
		last, loaded := m.lastProgress.LoadOrStore(obj.URL, -1)
		if !loaded || last.(int) != obj.GetProgress() {
			item := ProgressItem{
				TaskID:   obj.TaskID,
				URL:      obj.URL,
				Progress: obj.GetProgress(),
				Status:   obj.GetStatus(),
			}
			if obj.Metadata != nil {
				item.Title = obj.Metadata[model.MetadataKeyTitle]
			}
			batch.Updates = append(batch.Updates, item)
			m.lastProgress.Store(obj.URL, obj.GetProgress())
		}
		return true
	})
	if len(batch.Updates) > 0 {
		m.publish(core.Event{Type: core.EventProgressBatch, Payload: batch})
	}
}

func (m *Manager) BroadcastTaskUpdate(taskID string) {
	t, ok := m.getTask(taskID)

	if !ok {
		return
	}

	summary := map[string]any{
		"id":   taskID,
		"type": t.Type(),
	}

	if total, err := m.countTaskObjects(t, nil); err == nil {
		summary["total"] = total
	}
	if completed, err := m.countTaskObjects(t, &core.StorageQuery{
		Filter: core.StorageFilter{
			Statuses: []string{model.StatusCompleted},
		},
	}); err == nil {
		summary["completed"] = completed
	}
	{
		m.mu.Lock()
		summary["active"] = m.activeDownloads[taskID]
		m.mu.Unlock()
	}
	q := m.getTaskQueue(taskID)
	summary["backlog"] = len(q)
	if v, ok := m.metrics.Load(taskID); ok {
		mt := v.(*taskMetrics)
		summary["avg_latency_ms"] = mt.avgLatencyMs.Load()
		summary["failures"] = mt.failures.Load()
	}

	m.publish(core.Event{Type: core.EventTaskUpdate, Payload: summary})
}
