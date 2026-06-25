// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/core"
)

// ComponentHealth 描述单个组件的健康状态
type ComponentHealth struct {
	Status   string `json:"status"` // "ok" | "error" | "stopped"
	Detail   string `json:"detail,omitempty"`
	LastBeat string `json:"last_heartbeat,omitempty"` // RFC3339
}

// HealthStatus 描述整体健康状态
type HealthStatus struct {
	Status     string                     `json:"status"` // "ok" | "degraded" | "error"
	Uptime     string                     `json:"uptime"`
	Components map[string]ComponentHealth `json:"components"`
}

// GetHealthStatus 收集各组件健康状态并返回整体评估
func (m *Manager) GetHealthStatus() HealthStatus {
	hs := HealthStatus{
		Components: make(map[string]ComponentHealth),
		Uptime:     time.Since(m.startedAt).Round(time.Second).String(),
	}

	// Scheduler
	{
		v := m.schedulerHeartbeat.Load()
		lastBeat, ok := time.Time{}, false
		if v != nil {
			lastBeat, ok = v.(time.Time)
		}
		hs.Components["scheduler"] = checkHeartbeatComponent(m.schedulerEnabled.Load(), lastBeat, ok, "")
	}

	// Workers
	{
		v := m.workerHeartbeat.Load()
		lastBeat, ok := time.Time{}, false
		if v != nil {
			lastBeat, ok = v.(time.Time)
		}
		detail := fmt.Sprintf("%d workers", m.workerCount.Load())
		hs.Components["workers"] = checkHeartbeatComponent(m.workersEnabled.Load(), lastBeat, ok, detail)
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
	hs.Components["tasks"] = m.checkTasksComponent()

	// Overall status
	hs.Status = determineOverallStatus(hs.Components)
	return hs
}

const heartbeatTimeout = 5 * time.Second

// checkHeartbeatComponent 检查基于心跳的组件健康状态
func checkHeartbeatComponent(enabled bool, lastBeat time.Time, beatOK bool, okDetail string) ComponentHealth {
	c := ComponentHealth{}
	if !enabled {
		c.Status = "stopped"
		return c
	}
	c.Status = "error"
	c.Detail = "no heartbeat"
	if !beatOK {
		return c
	}
	c.LastBeat = lastBeat.Format(time.RFC3339)
	if time.Since(lastBeat) < heartbeatTimeout {
		c.Status = "ok"
		c.Detail = okDetail
	} else {
		c.Detail = fmt.Sprintf("last heartbeat %s ago", time.Since(lastBeat).Round(time.Second))
	}
	return c
}

// checkTasksComponent 收集任务组件的健康状态
func (m *Manager) checkTasksComponent() ComponentHealth {
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
	switch {
	case loaded == 0:
		c.Detail = "no tasks loaded"
	case okCount < loaded:
		c.Status = "degraded"
		c.Detail = fmt.Sprintf("%d/%d tasks have storage (%s)", okCount, loaded, strings.Join(failedTasks, ", "))
	default:
		c.Detail = fmt.Sprintf("%d tasks loaded", loaded)
	}
	return c
}

// determineOverallStatus 根据各组件状态计算整体健康等级
func determineOverallStatus(components map[string]ComponentHealth) string {
	overall := "ok"
	for _, c := range components {
		if c.Status == "error" {
			return "error"
		}
		if c.Status == "degraded" {
			overall = "degraded"
		}
	}
	return overall
}
