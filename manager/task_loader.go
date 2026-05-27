// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"log/slog"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/task"
)

func (m *Manager) loadTasks() {
	for _, tCfg := range m.currentCfg().Tasks {
		if _, exists := m.tasks.Load(tCfg.ID); exists {
			continue
		}
		if tCfg.Extra == nil {
			tCfg.Extra = make(map[string]any)
		}
		t, err := task.NewTask(&tCfg)
		if err != nil {
			slog.Error("Failed to create task", "task_id", tCfg.ID, "error", err)
			continue
		}
		t.SetDownloader(m.downloader)
		if srSetter, ok := t.(core.SharedRegistrySetter); ok {
			srSetter.SetSharedRegistry(m.urlRegistry)
		}
		if err = t.Start(); err != nil {
			slog.Error("Failed to start task", "task_id", tCfg.ID, "error", err)
			_ = t.Close()
			continue
		}
		m.urlRegistry.RegisterStorage(tCfg.ID, t.Storage())
		m.tasks.Store(tCfg.ID, t)
		slog.Info("Task loaded", "task_id", tCfg.ID)
	}
}

func (m *Manager) closeAllTasks() {
	tasks := make([]core.Task, 0, 64)
	m.tasks.Range(func(key, value any) bool {
		tasks = append(tasks, value.(core.Task))
		return true
	})
	for _, t := range tasks {
		if err := t.Close(); err != nil {
			slog.Error("Failed to close task", "task_id", t.ID(), "error", err)
		}
	}
}
