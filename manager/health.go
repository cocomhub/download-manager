// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/core"
)

// ComponentHealth 鎻忚堪鍗曚釜缁勪欢鐨勫仴搴风姸鎬?type ComponentHealth struct {
	Status   string `json:"status"` // "ok" | "error" | "stopped"
	Detail   string `json:"detail,omitempty"`
	LastBeat string `json:"last_heartbeat,omitempty"` // RFC3339
}

// HealthStatus 鎻忚堪鏁翠綋鍋ュ悍鐘舵€?type HealthStatus struct {
	Status     string                     `json:"status"` // "ok" | "degraded" | "error"
	Uptime     string                     `json:"uptime"`
	Components map[string]ComponentHealth `json:"components"`
}

// GetHealthStatus 鏀堕泦鍚勭粍浠跺仴搴风姸鎬佸苟杩斿洖鏁翠綋璇勪及
func (m *Manager) GetHealthStatus() HealthStatus {
	const heartbeatTimeout = 5 * time.Second
	hs := HealthStatus{
		Components: make(map[string]ComponentHealth),
		Uptime:     time.Since(m.startedAt).Round(time.Second).String(),
	}

	// Scheduler
	{
		c := ComponentHealth{}
		if m.schedulerEnabled.Load() {
			c.Status = "error"
			c.Detail = "no heartbeat"
			if v := m.schedulerHeartbeat.Load(); v != nil {
				if lastBeat, ok := v.(time.Time); ok {
					c.LastBeat = lastBeat.Format(time.RFC3339)
					if time.Since(lastBeat) < heartbeatTimeout {
						c.Status = "ok"
						c.Detail = ""
					} else {
						c.Detail = fmt.Sprintf("last heartbeat %s ago", time.Since(lastBeat).Round(time.Second))
					}
				}
			}
		} else {
			c.Status = "stopped"
		}
		hs.Components["scheduler"] = c
	}

	// Workers
	{
		c := ComponentHealth{}
		if m.workersEnabled.Load() {
			c.Status = "error"
			c.Detail = "no heartbeat"
			if v := m.workerHeartbeat.Load(); v != nil {
				if lastBeat, ok := v.(time.Time); ok {
					c.LastBeat = lastBeat.Format(time.RFC3339)
					if time.Since(lastBeat) < heartbeatTimeout {
						c.Status = "ok"
						c.Detail = fmt.Sprintf("%d workers", m.workerCount.Load())
					} else {
						c.Detail = fmt.Sprintf("last heartbeat %s ago", time.Since(lastBeat).Round(time.Second))
					}
				}
			}
		} else {
			c.Status = "stopped"
		}
		hs.Components["workers"] = c
	}

	// EventBus
	{
		m.eventMu.RLock()
		subCount := len(m.subscribers)
		m.eventMu.RUnlock()
		hs.Components["eventbus"] = ComponentHealth{
			Status: "ok",
			Detail: fmt.Sprintf("%d subscriber(s)", subCount),
		}
	}

	// Tasks
	{
		var loaded, okCount int
		var failedTasks []string
		m.tasks.Range(func(key, value any) bool {
			loaded++
			t := value.(core.Task)
			if t.Storage() != nil {
				okCount++
			} else {
				failedTasks = append(failedTasks, t.ID())
			}
			return true
		})
		c := ComponentHealth{Status: "ok"}
		if loaded == 0 {
			c.Detail = "no tasks loaded"
		} else if okCount < loaded {
			c.Status = "degraded"
			c.Detail = fmt.Sprintf("%d/%d tasks have storage (%s)", okCount, loaded, strings.Join(failedTasks, ", "))
		} else {
			c.Detail = fmt.Sprintf("%d tasks loaded", loaded)
		}
		hs.Components["tasks"] = c
	}

	// Overall status
	overall := "ok"
	for _, c := range hs.Components {
		if c.Status == "error" {
			overall = "error"
			break
		}
		if c.Status == "degraded" {
			overall = "degraded"
		}
	}
	hs.Status = overall
	return hs
}
