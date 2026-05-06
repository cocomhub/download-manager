// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"log/slog"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/storage"
	"github.com/cocomhub/download-manager/task"
)

func (m *Manager) loadTasks() {
	for _, tCfg := range m.currentCfg().Tasks {
		if _, exists := m.tasks.Load(tCfg.ID); exists {
			continue
		}
		storeType := tCfg.Storage.Type
		if storeType == "" {
			storeType = "memory"
		}
		store, err := storage.NewStorage(storeType, tCfg.Storage.Config)
		if err != nil {
			slog.Error("Failed to create storage for task", "task_id", tCfg.ID, "error", err)
			continue
		}
		if tCfg.Extra == nil {
			tCfg.Extra = make(map[string]any)
		}
		t, err := task.NewTask(tCfg, store)
		if err != nil {
			slog.Error("Failed to create task", "task_id", tCfg.ID, "error", err)
			continue
		}
		if setter, ok := t.(core.DownloaderSetter); ok {
			setter.SetDownloader(m.downloader)
		}
		if srSetter, ok := t.(core.SharedRegistrySetter); ok {
			srSetter.SetSharedRegistry(m.urlRegistry)
		}
		if sp, ok := t.(core.StorageProvider); ok {
			m.urlRegistry.RegisterStorage(tCfg.ID, sp.GetStorage())
		}
		m.tasks.Store(tCfg.ID, t)
		slog.Info("Task loaded", "task_id", tCfg.ID, "storage_type", storeType)
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
