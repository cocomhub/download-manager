// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"log/slog"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"
)

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
	m.downloadingObj.Range(func(key, value any) bool {
		obj := value.(*model.DownloadObject)

		// Check if progress has changed
		last, loaded := m.lastProgress.LoadOrStore(obj.URL, -1)
		if !loaded || last.(int) != obj.GetProgress() {
			m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
			m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
			m.lastProgress.Store(obj.URL, obj.GetProgress())
		}
		return true
	})
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
			Statuses: []string{dlcore.StatusCompleted},
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
