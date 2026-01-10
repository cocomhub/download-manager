package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"download-manager/config"
	"download-manager/core"
	"download-manager/downloader"
	"download-manager/logutil"
	"download-manager/model"
	"download-manager/storage"
	"download-manager/task"
)

type downloadRequest struct {
	task core.Task
	obj  *model.DownloadObject
}

type Manager struct {
	cfg         *config.Config
	tasks       map[string]core.Task
	downloader  core.Downloader
	stopChan    chan struct{}
	workerStop  chan struct{}
	workerCount int

	// Concurrency control
	activeDownloads map[string]int // TaskID -> Active Count (Just for stats/per-task limit if needed)
	mu              sync.Mutex
	downloadingObj  sync.Map // URL -> *model.DownloadObject (Active downloads)
	processingTask  sync.Map // TaskID -> bool (To track if task is being processed)
	failedCount     sync.Map // URL -> int (Failed download attempts)

	// Event Bus
	subscribers map[<-chan core.Event]chan core.Event
	eventMu     sync.RWMutex

	// Progress Deduplication
	lastProgress sync.Map // URL -> int

	// Global Rate Limiting
	downloadQueue chan *downloadRequest
	workerWg      sync.WaitGroup
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

	globalLimit := cfg.Downloader.GlobalConcurrent
	if globalLimit <= 0 {
		globalLimit = 5 // Default
	}

	return &Manager{
		cfg:             cfg,
		tasks:           make(map[string]core.Task),
		downloader:      downloader.New(cfg.Downloader),
		stopChan:        make(chan struct{}),
		workerStop:      make(chan struct{}),
		activeDownloads: make(map[string]int),
		downloadQueue:   make(chan *downloadRequest, max(globalLimit*2, 10)), // Buffer size
		subscribers:     make(map[<-chan core.Event]chan core.Event),
	}
}

func (m *Manager) GetDownloadRootDir() string {
	return filepath.Join(config.GetWorkDir(), "downloads")
}

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

func (m *Manager) Start() {
	slog.Info("Manager started")

	// Ensure work dir
	os.MkdirAll(filepath.Join(config.GetWorkDir(), "cache"), 0755)

	m.loadTasks()

	// Start Global Workers
	limit := m.cfg.Downloader.GlobalConcurrent
	if limit <= 0 {
		limit = 5
	}
	slog.Info("Starting global workers", "count", limit)
	for i := 0; i < limit; i++ {
		m.workerWg.Add(1)
		go m.worker()
	}
	m.workerCount = limit

	interval := time.Duration(m.cfg.TaskScan.Interval) * time.Second
	if interval == 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)

	// Save cache ticker
	cacheTicker := time.NewTicker(5 * time.Minute)

	// Progress broadcast ticker
	progressTicker := time.NewTicker(1 * time.Second)

	defer ticker.Stop()
	defer cacheTicker.Stop()
	defer progressTicker.Stop()

	// Immediate scan on start
	m.scan()

	for {
		select {
		case <-ticker.C:
			m.scan()
		case <-progressTicker.C:
			m.broadcastProgress()
		case <-cacheTicker.C:
			m.saveAllCaches()
		case <-m.stopChan:
			slog.Info("Manager stopping")
			// Close queue? Or just wait for context cancel if we had one.
			// Currently worker reads from queue forever.
			// We can close queue here but ensure no writes happen after.
			// m.scan happens in this loop, so no new writes from scan.
			// But RetryObject might write.
			m.saveAllCaches()
			return
		}
	}
}

func (m *Manager) Stop() {
	// Ideally close mongo clients here too, but they are global in storage pkg currently
	close(m.stopChan)
}

func (m *Manager) worker() {
	defer m.workerWg.Done()
	for {
		select {
		case req, ok := <-m.downloadQueue:
			if !ok {
				return
			}
			if req != nil {
				m.download(req.task, req.obj)
			}
		case <-m.stopChan:
			return
		case <-m.workerStop:
			return
		}
	}
}

func (m *Manager) loadTasks() {
	// Note: Caller must hold lock if concurrent access is possible
	// But Start calls it before ticker, so it's fine.
	// UpdateConfig calls it under lock.

	var wg sync.WaitGroup

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

		if tCfg.Extra == nil {
			tCfg.Extra = make(map[string]interface{})
		}

		// Create task using factory
		t, err := task.NewTask(tCfg, store)
		if err != nil {
			slog.Error("Failed to create task", "task_id", tCfg.ID, "error", err)
			continue
		}
		if setter, ok := t.(interface{ SetDownloader(core.Downloader) }); ok {
			setter.SetDownloader(m.downloader)
		}

		// Try load cache
		if ct, ok := t.(interface{ LoadCache() error }); ok {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := ct.LoadCache(); err != nil {
					slog.Warn("Failed to load task cache", "task_id", tCfg.ID, "error", err)
				} else {
					slog.Info("Task cache loaded", "task_id", tCfg.ID)
				}
			}()
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
		// Also close task to flush storage
		if err := t.Close(); err != nil {
			slog.Error("Failed to close task", "task_id", t.ID(), "error", err)
		}
	}
}

func (m *Manager) scan() {
	// slog.Debug("Scanning tasks")

	if m.cfg.TaskScan.Disable {
		return
	}

	m.mu.Lock()
	tasks := make([]core.Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasks = append(tasks, t)
	}
	m.mu.Unlock()

	for _, t := range tasks {
		// Check if task is already being processed
		if _, processing := m.processingTask.LoadOrStore(t.ID(), true); processing {
			continue
		}

		go m.processTask(t)
	}
}

func (m *Manager) processTask(t core.Task) {
	defer m.processingTask.Delete(t.ID())

	// Check per-task concurrency limit (soft limit for scheduling?)
	// If global limit is used, task limit might be redundant or acts as "fairness" limit.
	// Let's keep it.

	limit := 10 // Default limit
	if ct, ok := t.(interface{ GetConcurrency() int }); ok {
		limit = ct.GetConcurrency()
	}

	m.mu.Lock()
	active := m.activeDownloads[t.ID()]
	// If active >= limit, we stop scheduling new downloads for this task.
	if active >= limit {
		m.mu.Unlock()
		// slog.Debug("Task reached concurrency limit", "task_id", t.ID(), "active", active, "limit", limit)
		return
	}
	m.mu.Unlock()

	// Calculate remaining slots
	slotsAvailable := limit - active

	// Only fetch objects if we have capacity
	objs, err := t.GetDownloadObjects()
	if err != nil {
		slog.Error("Error getting objects for task", "task_id", t.ID(), "error", err)
		return
	}

	if len(objs) == 0 {
		return
	}
	// slog.Debug("Task has objects to download", "task_id", t.ID(), "count", len(objs))

	// Schedule downloads up to available slots
	count := 0

	for _, obj := range objs {
		if count >= slotsAvailable {
			break
		}

		if _, loaded := m.downloadingObj.LoadOrStore(obj.URL, obj); loaded { // Store obj instead of URL
			// slog.Debug("Object is already downloading", "task_id", t.ID(), "url", obj.URL)
			continue
		}

		// Attempt to push to global queue
		select {
		case m.downloadQueue <- &downloadRequest{task: t, obj: obj}:
			slog.Info("Object queued for download", "task_id", t.ID(), "url", obj.URL)

			m.mu.Lock()
			m.activeDownloads[t.ID()]++
			active++
			m.mu.Unlock()
			count++
			slotsAvailable--
		default:
			// Queue full, abort scheduling for now
			// Remove from downloadingObj map since we didn't schedule it
			m.downloadingObj.Delete(obj.URL)
		}
	}
	m.BroadcastTaskUpdate(t.ID())
}

func (m *Manager) broadcastProgress() {
	m.downloadingObj.Range(func(key, value interface{}) bool {
		obj := value.(*model.DownloadObject)

		// Check if progress has changed
		last, loaded := m.lastProgress.LoadOrStore(obj.URL, -1)
		if !loaded || last.(int) != obj.Progress {
			m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
			m.lastProgress.Store(obj.URL, obj.Progress)
		}
		return true
	})
}

func (m *Manager) BroadcastTaskUpdate(taskID string) {
	m.mu.Lock()
	t, ok := m.tasks[taskID]
	m.mu.Unlock()

	if !ok {
		return
	}

	summary := map[string]interface{}{
		"id":   taskID,
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

	m.publish(core.Event{Type: core.EventTaskUpdate, Payload: summary})
}

func (m *Manager) download(t core.Task, obj *model.DownloadObject) {
	defer func() {
		m.mu.Lock()
		m.activeDownloads[t.ID()]--
		m.mu.Unlock()

		// Remove from downloadingObj map
		m.downloadingObj.Delete(obj.URL)
		m.lastProgress.Delete(obj.URL)

		// Broadcast task update on finish
		m.BroadcastTaskUpdate(t.ID())
	}()

	t.UpdateStatus(obj, model.StatusDownloading, nil)
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})

	m.mu.Lock()
	dl := m.downloader
	m.mu.Unlock()

	err := dl.Download(obj)
	if err != nil {
		slog.Error("Download failed", "task_id", t.ID(), "url", obj.URL, "error", err)
		t.UpdateStatus(obj, model.StatusFailed, err)

		if errors.Is(err, downloader.ErrNoTry) {
			if ft, ok := t.(core.FailedTask); ok {
				ft.MarkAsFailed(obj, err)
			}
		}

		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return
		}

		// Increment failed count
		if count, ok := m.failedCount.LoadOrStore(obj.URL, 0); ok {
			m.failedCount.Store(obj.URL, count.(int)+1)
			// Check if max retries reached
			if count.(int)+1 >= 5 {
				if ft, ok := t.(core.FailedTask); ok {
					ft.MarkAsFailed(obj, fmt.Errorf("max retries reached: %w", err))
				}
			}
		}
	} else {
		t.UpdateStatus(obj, model.StatusCompleted, nil)
	}
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
}

// forceDownload bypasses the queue and runs immediately
func (m *Manager) forceDownload(t core.Task, obj *model.DownloadObject) {
	if _, loaded := m.downloadingObj.LoadOrStore(obj.URL, obj); loaded {
		return // Already downloading
	}

	slog.Info("Force starting download", "task_id", t.ID(), "url", obj.URL)

	m.mu.Lock()
	m.activeDownloads[t.ID()]++
	m.mu.Unlock()

	// Run in separate goroutine, bypassing worker pool limits
	go m.download(t, obj)
}

// New API methods
func (m *Manager) GetActiveDownloads() []map[string]interface{} {
	actives := make([]map[string]interface{}, 0)
	m.downloadingObj.Range(func(key, value interface{}) bool {
		obj := value.(*model.DownloadObject)
		actives = append(actives, map[string]interface{}{
			"task_id":  obj.TaskID,
			"url":      obj.URL,
			"title":    obj.Metadata["title"],
			"progress": obj.Progress,
			"status":   obj.Status, // Should be 'downloading'
		})
		return true
	})
	return actives
}

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

func (m *Manager) GetTaskDetails(id string, page, limit int, search, sortBy string) (map[string]interface{}, error) {
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

	// Task configuration exposure
	if getter, ok := t.(interface{ GetConcurrency() int }); ok {
		result["concurrency"] = getter.GetConcurrency()
	}
	if getter, ok := t.(interface{ GetRefreshInterval() int }); ok {
		result["refresh_interval"] = getter.GetRefreshInterval()
	}
	result["supports"] = map[string]bool{
		"concurrency":      hasMethod(t, "SetConcurrency"),
		"refresh_interval": hasMethod(t, "SetRefreshInterval"),
	}

	if st, ok := t.(interface {
		GetAllObjects() []*model.DownloadObject
	}); ok {
		objs := st.GetAllObjects()
		if objs == nil {
			objs = make([]*model.DownloadObject, 0)
		}

		// Filter by search query
		if search != "" {
			search = strings.ToLower(strings.TrimSpace(search))
			var filtered []*model.DownloadObject
			for _, obj := range objs {
				match := false
				if strings.Contains(strings.ToLower(obj.URL), search) {
					match = true
				} else if title, ok := obj.Metadata["title"]; ok && strings.Contains(strings.ToLower(title), search) {
					match = true
				} else if tags, ok := obj.Extra["tags"].([]interface{}); ok {
					for _, t := range tags {
						if tStr, ok := t.(string); ok && strings.Contains(strings.ToLower(tStr), search) {
							match = true
							break
						}
					}
				}

				if match {
					filtered = append(filtered, obj)
				}
			}
			objs = filtered
		}

		// Sort objects
		sort.Slice(objs, func(i, j int) bool {
			switch sortBy {
			case "date_asc":
				return objs[i].Metadata["date"] < objs[j].Metadata["date"]
			case "date_desc":
				return objs[i].Metadata["date"] > objs[j].Metadata["date"]
			case "name_asc":
				titleI := objs[i].Metadata["title"]
				if titleI == "" {
					titleI = objs[i].URL
				}
				titleJ := objs[j].Metadata["title"]
				if titleJ == "" {
					titleJ = objs[j].URL
				}
				return strings.ToLower(titleI) < strings.ToLower(titleJ)
			case "duration_desc":
				return objs[i].Metadata["duration"] > objs[j].Metadata["duration"]
			default:
				// Default: Date Desc, then URL Asc
				dateI := objs[i].Metadata["date"]
				dateJ := objs[j].Metadata["date"]
				if dateI != dateJ {
					return dateI > dateJ
				}
				return objs[i].URL < objs[j].URL
			}
		})

		total := len(objs)
		var pagedObjs []*model.DownloadObject

		if limit <= 0 {
			// All
			pagedObjs = objs
			page = 1
			limit = total
		} else {
			if page < 1 {
				page = 1
			}
			start := (page - 1) * limit
			if start > total {
				start = total
			}
			end := start + limit
			if end > total {
				end = total
			}
			pagedObjs = objs[start:end]
		}

		result["objects"] = pagedObjs
		result["total"] = total
		result["page"] = page
		result["limit"] = limit
	}

	return result, nil
}

func hasMethod(i interface{}, name string) bool {
	// lightweight capability hint via type assertion
	switch name {
	case "SetConcurrency":
		_, ok := i.(interface{ SetConcurrency(int) error })
		return ok
	case "SetRefreshInterval":
		_, ok := i.(interface{ SetRefreshInterval(int) error })
		return ok
	default:
		return false
	}
}

// RetryObject resets the status of an object to pending and forces download
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

				// Resolve details if needed (JIT for forced retry?)
				if resolver, ok := t.(interface {
					ResolveObject(*model.DownloadObject) error
				}); ok {
					slog.Info("Resolving object before retry", "task_id", taskID, "url", url)
					if err := resolver.ResolveObject(obj); err != nil {
						slog.Error("Failed to resolve object for retry", "error", err)
						return fmt.Errorf("failed to resolve object: %v", err)
					}
				}

				m.forceDownload(t, obj)
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
				// Should we force download all? That might be too many.
				// Just let them be picked up by scan.
				count++
			}
		}
		if count > 0 {
			// Trigger scan
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

	// Validate and clamp
	newCfg.ValidateAndClamp()

	// Save to file
	err := config.Save(config.GetConfigFilePath(), newCfg)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	// Write backup with timestamp
	if err := m.writeConfigBackup(newCfg); err != nil {
		slog.Warn("Failed to write config backup", "error", err)
	}

	// Update internal config
	m.cfg = newCfg

	// Reload Downloader
	m.downloader = downloader.New(newCfg.Downloader)

	// Reload Logger
	logutil.InitLogger(newCfg.Log)

	// Resize worker pool dynamically
	newLimit := newCfg.Downloader.GlobalConcurrent
	if newLimit <= 0 {
		newLimit = 5
	}
	if newLimit > m.workerCount {
		add := newLimit - m.workerCount
		slog.Info("Increasing global workers", "from", m.workerCount, "to", newLimit)
		for i := 0; i < add; i++ {
			m.workerWg.Add(1)
			go m.worker()
		}
		m.workerCount = newLimit
	} else if newLimit < m.workerCount {
		remove := m.workerCount - newLimit
		slog.Info("Decreasing global workers", "from", m.workerCount, "to", newLimit)
		for i := 0; i < remove; i++ {
			select {
			case m.workerStop <- struct{}{}:
			default:
				// ensure non-blocking if no worker is waiting; still attempt send
				m.workerStop <- struct{}{}
			}
		}
		m.workerCount = newLimit
	}

	// Task-level dynamic updates (no rebuild)
	for _, tCfg := range newCfg.Tasks {
		if t, ok := m.tasks[tCfg.ID]; ok {
			// Concurrency setter
			if setter, ok := t.(interface{ SetConcurrency(int) error }); ok {
				// detect change
				var cfgVal int
				if tCfg.Extra != nil {
					if v, ok := tCfg.Extra["max_concurrent"].(int); ok {
						cfgVal = v
					} else if v, ok := tCfg.Extra["max_concurrent"].(float64); ok {
						cfgVal = int(v)
					}
				}
				if cfgVal > 0 {
					if getter, ok := t.(interface{ GetConcurrency() int }); !ok || getter.GetConcurrency() != cfgVal {
						if err := setter.SetConcurrency(cfgVal); err != nil {
							slog.Warn("SetConcurrency failed", "task_id", tCfg.ID, "error", err)
						} else {
							slog.Info("Task concurrency updated", "task_id", tCfg.ID, "value", cfgVal)
						}
					}
				}
			}
			// Refresh interval setter
			if setter, ok := t.(interface{ SetRefreshInterval(int) error }); ok {
				var cfgVal int
				if tCfg.Extra != nil {
					if v, ok := tCfg.Extra["refresh_interval"].(int); ok {
						cfgVal = v
					} else if v, ok := tCfg.Extra["refresh_interval"].(float64); ok {
						cfgVal = int(v)
					}
				}
				if cfgVal > 0 {
					if getter, ok := t.(interface{ GetRefreshInterval() int }); !ok || getter.GetRefreshInterval() != cfgVal {
						if err := setter.SetRefreshInterval(cfgVal); err != nil {
							slog.Warn("SetRefreshInterval failed", "task_id", tCfg.ID, "error", err)
						} else {
							slog.Info("Task refresh interval updated", "task_id", tCfg.ID, "value", cfgVal)
						}
					}
				}
			}
		}
	}

	// Load only missing tasks (existing keep state)
	m.loadTasks()

	slog.Info("Configuration updated")
	m.publish(core.Event{Type: core.EventTaskListChange, Payload: nil})
	go m.scan()
	return nil
}

func (m *Manager) UpdateLogConfig(newLog logutil.LogConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg.Log = newLog
	m.cfg.ValidateAndClamp()
	if err := config.Save(config.GetConfigFilePath(), m.cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	logutil.InitLogger(newLog)
	return nil
}

func (m *Manager) writeConfigBackup(cfg *config.Config) error {
	dir := filepath.Join(config.GetWorkDir(), "config_backups")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	name := fmt.Sprintf("config_%s.yaml", time.Now().Format("20060102_150405"))
	path := filepath.Join(dir, name)
	return config.Save(path, cfg)
}

func (m *Manager) ListConfigBackups() ([]map[string]string, error) {
	dir := filepath.Join(config.GetWorkDir(), "config_backups")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []map[string]string{}, nil
	}
	var res []map[string]string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "config_") && strings.HasSuffix(name, ".yaml") {
			item := map[string]string{
				"filename": name,
				"path":     filepath.Join(dir, name),
			}
			metaPath := filepath.Join(dir, name+".meta.json")
			if data, err := os.ReadFile(metaPath); err == nil {
				item["meta"] = string(data)
			}
			res = append(res, item)
		}
	}
	// Sort newest first by filename suffix timestamp
	sort.Slice(res, func(i, j int) bool {
		return res[i]["filename"] > res[j]["filename"]
	})
	return res, nil
}

func (m *Manager) RollbackConfig(filename string) error {
	dir := filepath.Join(config.GetWorkDir(), "config_backups")
	path := filepath.Join(dir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("invalid backup content: %w", err)
	}
	return m.UpdateConfig(&cfg)
}

func (m *Manager) DiffConfigFiles(left, right string) (map[string]interface{}, error) {
	var leftCfg, rightCfg config.Config
	var leftYml, rightYml []byte
	var err error
	if left == "current" || left == "" {
		m.mu.Lock()
		leftCfg = *m.cfg
		m.mu.Unlock()
		leftYml, _ = yaml.Marshal(leftCfg)
	} else {
		lp := filepath.Join(config.GetWorkDir(), "config_backups", left)
		leftYml, err = os.ReadFile(lp)
		if err != nil {
			return nil, fmt.Errorf("read left backup failed: %w", err)
		}
		if err := yaml.Unmarshal(leftYml, &leftCfg); err != nil {
			return nil, fmt.Errorf("parse left backup failed: %w", err)
		}
	}
	if right == "current" || right == "" {
		m.mu.Lock()
		rightCfg = *m.cfg
		m.mu.Unlock()
		rightYml, _ = yaml.Marshal(rightCfg)
	} else {
		rp := filepath.Join(config.GetWorkDir(), "config_backups", right)
		rightYml, err = os.ReadFile(rp)
		if err != nil {
			return nil, fmt.Errorf("read right backup failed: %w", err)
		}
		if err := yaml.Unmarshal(rightYml, &rightCfg); err != nil {
			return nil, fmt.Errorf("parse right backup failed: %w", err)
		}
	}
	diff := leftCfg.Diff(rightCfg)
	return map[string]interface{}{
		"left":       left,
		"right":      right,
		"left_yaml":  string(leftYml),
		"right_yaml": string(rightYml),
		"changes":    diff,
	}, nil
}

func normalizeYAML(src string, ignoreWS, ignoreComments bool) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if ignoreComments {
			trim := strings.TrimSpace(l)
			if strings.HasPrefix(trim, "#") {
				continue
			}
		}
		if ignoreWS {
			l = strings.TrimRight(l, " \t")
			l = strings.ReplaceAll(l, "\t", "    ")
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func (m *Manager) DiffConfigFilesOpts(left, right string, ignoreWS, ignoreComments bool) (map[string]interface{}, error) {
	res, err := m.DiffConfigFiles(left, right)
	if err != nil {
		return nil, err
	}
	if ignoreWS || ignoreComments {
		res["left_norm"] = normalizeYAML(res["left_yaml"].(string), ignoreWS, ignoreComments)
		res["right_norm"] = normalizeYAML(res["right_yaml"].(string), ignoreWS, ignoreComments)
	}
	return res, nil
}

func (m *Manager) AddConfigTag(filename, tag string) error {
	if tag == "" {
		return fmt.Errorf("tag is empty")
	}
	dir := filepath.Join(config.GetWorkDir(), "config_backups")
	meta := filepath.Join(dir, filename+".meta.json")
	var tags []string
	if data, err := os.ReadFile(meta); err == nil {
		var obj struct {
			Tags []string `json:"tags"`
		}
		_ = json.Unmarshal(data, &obj)
		tags = obj.Tags
	}
	for _, t := range tags {
		if t == tag {
			return nil
		}
	}
	tags = append(tags, tag)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]interface{}{"tags": tags})
	return os.WriteFile(meta, buf.Bytes(), 0644)
}

func (m *Manager) AddConfigNote(filename, message, author string) error {
	if message == "" {
		return fmt.Errorf("message is empty")
	}
	dir := filepath.Join(config.GetWorkDir(), "config_backups")
	meta := filepath.Join(dir, filename+".meta.json")
	var obj struct {
		Tags  []string `json:"tags"`
		Notes []struct {
			Message   string `json:"message"`
			Author    string `json:"author"`
			Timestamp int64  `json:"timestamp"`
		} `json:"notes"`
	}
	if data, err := os.ReadFile(meta); err == nil {
		_ = json.Unmarshal(data, &obj)
	}
	obj.Notes = append(obj.Notes, struct {
		Message   string `json:"message"`
		Author    string `json:"author"`
		Timestamp int64  `json:"timestamp"`
	}{Message: message, Author: author, Timestamp: time.Now().Unix()})
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(obj)
	return os.WriteFile(meta, buf.Bytes(), 0644)
}

func (m *Manager) SetTaskConfig(taskID string, concurrency *int, refreshInterval *int) (map[string]bool, error) {
	m.mu.Lock()
	t, ok := m.tasks[taskID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	result := map[string]bool{"concurrency": false, "refresh_interval": false}
	if concurrency != nil {
		if setter, ok := t.(interface{ SetConcurrency(int) error }); ok {
			if err := setter.SetConcurrency(*concurrency); err != nil {
				return result, err
			}
			result["concurrency"] = true
		}
	}
	if refreshInterval != nil {
		if setter, ok := t.(interface{ SetRefreshInterval(int) error }); ok {
			if err := setter.SetRefreshInterval(*refreshInterval); err != nil {
				return result, err
			}
			result["refresh_interval"] = true
		}
	}
	return result, nil
}
