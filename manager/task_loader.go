package manager

import (
	"log/slog"
	"sync"

	"download-manager/core"
	"download-manager/storage"
	"download-manager/task"
)

func (m *Manager) loadTasks() {
	var wg sync.WaitGroup
	for _, tCfg := range m.cfg.Tasks {
		if _, exists := m.tasks[tCfg.ID]; exists {
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
			tCfg.Extra = make(map[string]interface{})
		}
		t, err := task.NewTask(tCfg, store)
		if err != nil {
			slog.Error("Failed to create task", "task_id", tCfg.ID, "error", err)
			continue
		}
		if setter, ok := t.(interface{ SetDownloader(core.Downloader) }); ok {
			setter.SetDownloader(m.downloader)
		}
		if ct, ok := t.(interface{ LoadCache() error }); ok {
			wg.Add(1)
			go func(id string, ct interface{ LoadCache() error }) {
				defer wg.Done()
				if err := ct.LoadCache(); err != nil {
					slog.Warn("Failed to load task cache", "task_id", id, "error", err)
				} else {
					slog.Info("Task cache loaded", "task_id", id)
				}
			}(tCfg.ID, ct)
		}
		m.tasks[tCfg.ID] = t
		slog.Info("Task loaded", "task_id", tCfg.ID, "storage_type", storeType)
	}
	wg.Wait()
}

func (m *Manager) saveAllCaches() {
	m.mu.Lock()
	tasks := make([]core.Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasks = append(tasks, t)
	}
	m.mu.Unlock()
	for _, t := range tasks {
		if ct, ok := t.(interface{ SaveCache() error }); ok {
			if err := ct.SaveCache(); err != nil {
				slog.Error("Failed to save task cache", "task_id", t.ID(), "error", err)
			}
		}
		if err := t.Close(); err != nil {
			slog.Error("Failed to close task", "task_id", t.ID(), "error", err)
		}
	}
}

