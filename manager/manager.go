package manager

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"download-manager/config"
	"download-manager/core"
	"download-manager/downloader"
	"download-manager/model"
	"download-manager/storage"
	"download-manager/task"
)

type Manager struct {
	cfg        *config.Config
	tasks      map[string]core.Task
	downloader core.Downloader
	stopChan   chan struct{}

	// Concurrency control
	activeDownloads map[string]int // TaskID -> Active Count
	mu              sync.Mutex
	downloadingObj  sync.Map
}

func NewManager(cfg *config.Config) *Manager {
	// Initialize Mongo Clients if configured
	var mongoConfigs []struct{ Name, URI string }
	for _, m := range cfg.Mongo {
		mongoConfigs = append(mongoConfigs, struct{ Name, URI string }{m.Name, m.URI})
	}
	if len(mongoConfigs) > 0 {
		if err := storage.InitMongoClients(mongoConfigs); err != nil {
			slog.Warn("Failed to init mongo clients", "error", err)
		}
	}

	return &Manager{
		cfg:             cfg,
		tasks:           make(map[string]core.Task),
		downloader:      downloader.NewWgetDownloader(cfg.Downloader),
		stopChan:        make(chan struct{}),
		activeDownloads: make(map[string]int),
	}
}

func (m *Manager) Start() {
	slog.Info("Manager started")
	m.loadTasks()

	interval := time.Duration(m.cfg.Server.ScanInterval) * time.Second
	if interval == 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Immediate scan on start
	m.scan()

	for {
		select {
		case <-ticker.C:
			m.scan()
		case <-m.stopChan:
			slog.Info("Manager stopping")
			return
		}
	}
}

func (m *Manager) Stop() {
	// Ideally close mongo clients here too, but they are global in storage pkg currently
	close(m.stopChan)
}

func (m *Manager) loadTasks() {
	// Note: Caller must hold lock if concurrent access is possible
	// But Start calls it before ticker, so it's fine.
	// UpdateConfig calls it under lock.

	for _, tCfg := range m.cfg.Tasks {
		if _, exists := m.tasks[tCfg.ID]; exists {
			continue
		}

		// Create storage
		// If storage type is not specified, default to memory (transient)
		storeType := tCfg.Storage.Type
		if storeType == "" {
			storeType = "memory"
		}

		store, err := storage.NewStorage(storeType, tCfg.Storage.Config)
		if err != nil {
			slog.Error("Failed to create storage for task", "task_id", tCfg.ID, "error", err)
			continue
		}

		// Create task using factory
		t, err := task.NewTask(tCfg, store)
		if err != nil {
			slog.Error("Failed to create task", "task_id", tCfg.ID, "error", err)
			continue
		}

		m.tasks[tCfg.ID] = t
		slog.Info("Task loaded", "task_id", tCfg.ID, "storage_type", storeType)
	}
}

func (m *Manager) scan() {
	slog.Debug("Scanning tasks")

	m.mu.Lock()
	tasks := make([]core.Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasks = append(tasks, t)
	}
	m.mu.Unlock()

	for _, t := range tasks {
		go m.processTask(t)
	}
}

func (m *Manager) processTask(t core.Task) {
	// Check concurrency limit
	m.mu.Lock()
	active := m.activeDownloads[t.ID()]
	m.mu.Unlock()

	limit := 10 // Default limit
	// Check if task supports concurrency limit
	if ct, ok := t.(interface{ GetConcurrency() int }); ok {
		limit = ct.GetConcurrency()
	}

	slog.Debug("Task concurrency", "task_id", t.ID(), "active", active, "limit", limit)

	// If active >= limit, we stop scheduling new downloads.
	if active >= limit {
		slog.Debug("Task reached concurrency limit", "task_id", t.ID(), "active", active, "limit", limit)
		return
	}

	// Calculate remaining slots
	slotsAvailable := limit - active

	// Only fetch objects if we have capacity
	objs, err := t.GetDownloadObjects()
	if err != nil {
		slog.Error("Error getting objects for task", "task_id", t.ID(), "error", err)
		return
	}
	slog.Debug("Task has objects to download", "task_id", t.ID(), "count", len(objs))

	if len(objs) == 0 {
		return
	}

	// Schedule downloads up to available slots
	count := 0

	for _, obj := range objs {
		if count >= slotsAvailable {
			break
		}

		if _, loaded := m.downloadingObj.LoadOrStore(obj.URL, obj.URL); loaded {
			slog.Debug("Object is already downloading", "task_id", t.ID(), "url", obj.URL)
			continue
		}

		slog.Info("Object scheduled for download", "task_id", t.ID(), "url", obj.URL)

		m.mu.Lock()
		m.activeDownloads[t.ID()]++
		active++ // Local counter update
		m.mu.Unlock()

		go m.download(t, obj)
		count++

		// Update slots locally
		slotsAvailable--
	}

	if count > 0 {
		slog.Info("Task scheduled new downloads", "task_id", t.ID(), "count", count)
	}
}

func (m *Manager) download(t core.Task, obj *model.DownloadObject) {
	defer func() {
		m.mu.Lock()
		m.activeDownloads[t.ID()]--
		m.mu.Unlock()

		// Remove from downloadingObj map
		m.downloadingObj.Delete(obj.URL)
	}()

	t.UpdateStatus(obj, model.StatusDownloading, nil)

	// Access downloader safely?
	// In UpdateConfig we replace m.downloader.
	// Since we don't lock here, it might be racey.
	// But replacing interface value is atomic-ish on some archs, but not guaranteed.
	// Let's grab lock to get downloader reference.
	m.mu.Lock()
	dl := m.downloader
	m.mu.Unlock()

	err := dl.Download(obj)
	if err != nil {
		t.UpdateStatus(obj, model.StatusFailed, err)
	} else {
		t.UpdateStatus(obj, model.StatusCompleted, nil)
	}
}

// New API methods
func (m *Manager) GetTaskSummaries() []map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	var summaries []map[string]interface{}
	// Iterate using config order to maintain consistency
	for _, tCfg := range m.cfg.Tasks {
		id := tCfg.ID
		t, ok := m.tasks[id]
		if !ok {
			continue
		}

		summary := map[string]interface{}{
			"id":   id,
			"type": t.Type(),
		}

		if st, ok := t.(interface {
			GetAllObjects() []*model.DownloadObject
		}); ok {
			objs := st.GetAllObjects()
			summary["total"] = len(objs)
			completed := 0
			for _, o := range objs {
				if o.Status == model.StatusCompleted {
					completed++
				}
			}
			summary["completed"] = completed
		}

		summaries = append(summaries, summary)
	}
	return summaries
}

func (m *Manager) GetTaskDetails(id string) (map[string]interface{}, error) {
	m.mu.Lock()
	t, ok := m.tasks[id]
	m.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("task not found")
	}

	result := map[string]interface{}{
		"id":   t.ID(),
		"type": t.Type(),
	}

	if st, ok := t.(interface {
		GetAllObjects() []*model.DownloadObject
	}); ok {
		result["objects"] = st.GetAllObjects()
	}

	return result, nil
}

// RetryObject resets the status of an object to pending
func (m *Manager) RetryObject(taskID, url string) error {
	m.mu.Lock()
	t, ok := m.tasks[taskID]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("task not found")
	}

	if st, ok := t.(interface {
		GetAllObjects() []*model.DownloadObject
	}); ok {
		objs := st.GetAllObjects()
		for _, obj := range objs {
			if obj.URL == url {
				if obj.Status == model.StatusCompleted {
					return fmt.Errorf("object already completed")
				}
				// Reset status
				t.UpdateStatus(obj, model.StatusPending, nil)
				obj.Progress = 0
				// Trigger scan immediately
				go m.processTask(t)
				return nil
			}
		}
		return fmt.Errorf("object not found")
	}
	return fmt.Errorf("task does not support object access")
}

// ReorderObject moves an object to a new position
func (m *Manager) ReorderObject(taskID, url string, newIndex int) error {
	m.mu.Lock()
	t, ok := m.tasks[taskID]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("task not found")
	}

	if st, ok := t.(interface {
		SetObjectIndex(url string, newIndex int) error
	}); ok {
		return st.SetObjectIndex(url, newIndex)
	}
	return fmt.Errorf("task does not support reordering")
}

// RetryAllFailed resets all failed objects in a task
func (m *Manager) RetryAllFailed(taskID string) error {
	m.mu.Lock()
	t, ok := m.tasks[taskID]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("task not found")
	}

	if st, ok := t.(interface {
		GetAllObjects() []*model.DownloadObject
	}); ok {
		objs := st.GetAllObjects()
		count := 0
		for _, obj := range objs {
			if obj.Status == model.StatusFailed {
				t.UpdateStatus(obj, model.StatusPending, nil)
				obj.Progress = 0
				count++
			}
		}
		if count > 0 {
			go m.processTask(t)
		}
		return nil
	}
	return fmt.Errorf("task does not support object access")
}

// --- Config Management ---

func (m *Manager) GetConfig() *config.Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

func (m *Manager) UpdateConfig(newCfg *config.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Save to file
	data, err := yaml.Marshal(newCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile("config.yaml", data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Update internal config
	m.cfg = newCfg

	// Reload Downloader
	m.downloader = downloader.NewWgetDownloader(newCfg.Downloader)

	// Reload Tasks (Add new ones)
	// We call loadTasks logic directly here since we hold lock and loadTasks expects it?
	// Actually loadTasks was designed to run without lock or before start.
	// But it accesses m.tasks.
	// Let's duplicate the logic or extract it safely.
	// Since we hold m.mu, we can just update m.tasks.

	for _, tCfg := range m.cfg.Tasks {
		if _, exists := m.tasks[tCfg.ID]; exists {
			continue
		}

		// Create storage
		storeType := tCfg.Storage.Type
		if storeType == "" {
			storeType = "memory"
		}

		store, err := storage.NewStorage(storeType, tCfg.Storage.Config)
		if err != nil {
			slog.Error("Failed to create storage for task", "task_id", tCfg.ID, "error", err)
			continue
		}

		// Create task using factory
		t, err := task.NewTask(tCfg, store)
		if err != nil {
			slog.Error("Failed to create task", "task_id", tCfg.ID, "error", err)
			continue
		}

		m.tasks[tCfg.ID] = t
		slog.Info("Task loaded", "task_id", tCfg.ID, "storage_type", storeType)
	}

	slog.Info("Configuration updated")
	return nil
}
