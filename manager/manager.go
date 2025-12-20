package manager

import (
	"fmt"
	"sync"
	"time"

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
			fmt.Printf("Warning: Failed to init mongo clients: %v\n", err)
		}
	}

	return &Manager{
		cfg:             cfg,
		tasks:           make(map[string]core.Task),
		downloader:      downloader.NewProxyWgetDownloader(cfg.Downloader),
		stopChan:        make(chan struct{}),
		activeDownloads: make(map[string]int),
	}
}

func (m *Manager) Start() {
	fmt.Println("Manager started...")
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
			fmt.Println("Manager stopping...")
			return
		}
	}
}

func (m *Manager) Stop() {
	// Ideally close mongo clients here too, but they are global in storage pkg currently
	close(m.stopChan)
}

func (m *Manager) loadTasks() {
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
			fmt.Printf("Failed to create storage for task %s: %v\n", tCfg.ID, err)
			continue
		}

		// Create task using factory
		t, err := task.NewTask(tCfg, store)
		if err != nil {
			fmt.Printf("Failed to create task %s: %v\n", tCfg.ID, err)
			continue
		}

		m.tasks[tCfg.ID] = t
		fmt.Printf("Task loaded: %s (Storage: %s)\n", tCfg.ID, storeType)
	}
}

func (m *Manager) scan() {
	// fmt.Println("Scanning tasks...") // Reduce log noise
	for _, t := range m.tasks {
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

	fmt.Printf("Task %s concurrency limit (%d/%d)\n", t.ID(), active, limit)

	if active >= limit {
		fmt.Printf("Task %s reached concurrency limit (%d/%d)\n", t.ID(), active, limit)
		return
	}

	// Only fetch objects if we have capacity
	objs, err := t.GetDownloadObjects()
	if err != nil {
		fmt.Printf("Error getting objects for task %s: %v\n", t.ID(), err)
		return
	}
	fmt.Printf("Task %s has %d objects to download\n", t.ID(), len(objs))

	if len(objs) == 0 {
		return
	}

	// Schedule downloads up to limit
	slotsAvailable := limit - active
	count := 0

	for _, obj := range objs {
		if count >= slotsAvailable {
			break
		}

		if _, loaded := m.downloadingObj.LoadOrStore(obj.URL, obj.URL); loaded {
			fmt.Printf("Task %s object %s is already downloading\n", t.ID(), obj.URL)
			continue
		}
		fmt.Printf("Task %s object %s is scheduled for download\n", t.ID(), obj.URL)

		m.mu.Lock()
		m.activeDownloads[t.ID()]++
		m.mu.Unlock()

		go m.download(t, obj)
		count++
	}

	if count > 0 {
		fmt.Printf("Task %s scheduled %d new downloads\n", t.ID(), count)
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
	err := m.downloader.Download(obj)
	if err != nil {
		t.UpdateStatus(obj, model.StatusFailed, err)
	} else {
		t.UpdateStatus(obj, model.StatusCompleted, nil)
	}
}

// New API methods
func (m *Manager) GetTaskSummaries() []map[string]interface{} {
	var summaries []map[string]interface{}
	for id, t := range m.tasks {
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
	t, ok := m.tasks[id]
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
