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
			tCfg.Extra = make(map[string]interface{})
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
		if ct, ok := t.(core.CacheLoader); ok {
			wg.Add(1)
			go func(id string, ct core.CacheLoader) {
				defer wg.Done()
				if err := ct.LoadCache(); err != nil {
					slog.Warn("Failed to load task cache", "task_id", id, "error", err)
				} else {
					slog.Info("Task cache loaded", "task_id", id)
				}
			}(tCfg.ID, ct)
		}
		m.tasks.Store(tCfg.ID, t)
		slog.Info("Task loaded", "task_id", tCfg.ID, "storage_type", storeType)
	}
	wg.Wait()
	// Cold start rehydration from storages
	m.tasks.Range(func(key, value any) bool {
		if sp, ok := value.(core.StorageProvider); ok {
			st := sp.GetStorage()
			if st != nil {
				if list, err := st.Search(nil); err == nil && list != nil {
					for _, obj := range list {
						m.urlRegistry.Update(obj)
					}
				}
			}
		}
		return true
	})
}

func (m *Manager) saveAllCaches() {
	tasks := make([]core.Task, 0, 64)
	m.tasks.Range(func(key, value any) bool {
		tasks = append(tasks, value.(core.Task))
		return true
	})
	for _, t := range tasks {
		if ct, ok := t.(core.CacheSaver); ok {
			if err := ct.SaveCache(); err != nil {
				slog.Error("Failed to save task cache", "task_id", t.ID(), "error", err)
			}
		}
		if err := t.Close(); err != nil {
			slog.Error("Failed to close task", "task_id", t.ID(), "error", err)
		}
	}
}
