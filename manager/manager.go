package manager

import (
	"fmt"
	"time"

	"download-manager/config"
	"download-manager/core"
	"download-manager/downloader"
	"download-manager/model"
	"download-manager/task"
)

type Manager struct {
	cfg        *config.Config
	tasks      map[string]core.Task
	downloader core.Downloader
	stopChan   chan struct{}
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:        cfg,
		tasks:      make(map[string]core.Task),
		downloader: downloader.NewWgetDownloader(),
		stopChan:   make(chan struct{}),
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
	close(m.stopChan)
}

func (m *Manager) loadTasks() {
	for _, tCfg := range m.cfg.Tasks {
		if _, exists := m.tasks[tCfg.ID]; exists {
			continue
		}

		// Factory pattern could be used here
		if tCfg.Type == "simple_url_list" {
			t := task.NewSimpleTask(tCfg.ID, tCfg.URLs, tCfg.SaveDir)
			m.tasks[tCfg.ID] = t
			fmt.Printf("Task loaded: %s\n", tCfg.ID)
		}
	}
}

func (m *Manager) scan() {
	fmt.Println("Scanning tasks...")
	for _, t := range m.tasks {
		go m.processTask(t)
	}
}

func (m *Manager) processTask(t core.Task) {
	objs, err := t.GetDownloadObjects()
	if err != nil {
		fmt.Printf("Error getting objects for task %s: %v\n", t.ID(), err)
		return
	}

	if len(objs) == 0 {
		return
	}

	fmt.Printf("Task %s has %d pending objects\n", t.ID(), len(objs))

	for _, obj := range objs {
		// In a real system, we would use a worker pool/semaphore here
		// to limit concurrent downloads.
		go m.download(t, obj)
	}
}

func (m *Manager) download(t core.Task, obj *model.DownloadObject) {
	t.UpdateStatus(obj, model.StatusDownloading, nil)
	err := m.downloader.Download(obj)
	if err != nil {
		t.UpdateStatus(obj, model.StatusFailed, err)
	} else {
		t.UpdateStatus(obj, model.StatusCompleted, nil)
	}
}
