// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/downloader"
	"github.com/cocomhub/download-manager/logutil"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
)

type downloadRequest struct {
	task core.Task
	obj  *model.DownloadObject
}

type Manager struct {
	cfg            *config.Config
	cfgVal         atomic.Value
	tasks          sync.Map
	downloader     core.Downloader
	stopChan       chan struct{}
	workerStop     chan struct{}
	workerCount    int
	taskLocks      sync.Map
	lastBackupName string
	taskQueues     sync.Map
	schedulerStop  chan struct{}

	// Concurrency control
	activeDownloads map[string]int // TaskID -> Active Count (Just for stats/per-task limit if needed)
	mu              sync.Mutex
	downloadingObj  sync.Map // URL -> *model.DownloadObject (Active downloads)
	processingTask  sync.Map // TaskID -> bool (To track if task is being processed)
	failedCount     sync.Map // URL -> int (Failed download attempts)
	metrics         sync.Map // TaskID -> *taskMetrics

	// Event Bus
	subscribers map[<-chan core.Event]chan core.Event
	eventMu     sync.RWMutex

	// Progress Deduplication
	lastProgress sync.Map // URL -> int

	// Global Rate Limiting
	downloadQueue chan *downloadRequest
	workerWg      sync.WaitGroup

	// Global shared URL state registry
	urlRegistry *URLStateRegistry
}

type taskMetrics struct {
	avgLatencyMs float64
	failures     int
	completed    int
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

	mgr := &Manager{
		cfg:             cfg,
		downloader:      downloader.New(cfg.Downloader),
		stopChan:        make(chan struct{}),
		workerStop:      make(chan struct{}),
		activeDownloads: make(map[string]int),
		downloadQueue:   make(chan *downloadRequest, max(globalLimit*2, 10)), // Buffer size
		subscribers:     make(map[<-chan core.Event]chan core.Event),
		urlRegistry:     NewURLStateRegistry(),
	}
	mgr.cfgVal.Store(cfg)
	if nd, ok := mgr.downloader.(*downloader.NativeHTTPDownloader); ok {
		nd.ApplyDomainLimits(cfg.Downloader.DomainLimits)
	}
	return mgr
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

func (m *Manager) currentCfg() *config.Config {
	if v := m.cfgVal.Load(); v != nil {
		return v.(*config.Config)
	}
	return m.cfg
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
	// Start fair scheduler
	m.schedulerStop = make(chan struct{})
	go m.scheduler()

	interval := time.Duration(m.currentCfg().TaskScan.Interval) * time.Second
	if interval == 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)

	// Save cache ticker
	cacheTicker := time.NewTicker(5 * time.Minute)

	// Progress broadcast ticker
	progressTicker := time.NewTicker(1 * time.Second)
	alignTicker := time.NewTicker(5 * time.Minute)

	defer ticker.Stop()
	defer cacheTicker.Stop()
	defer progressTicker.Stop()
	defer alignTicker.Stop()

	// Immediate scan on start
	m.scan()

	for {
		select {
		case <-ticker.C:
			m.scan()
		case <-progressTicker.C:
			m.broadcastProgress()
		case <-cacheTicker.C:
			m.saveAllCaches(false)
		case <-alignTicker.C:
			m.alignStorages()
		case <-m.stopChan:
			slog.Info("Manager stopping")
			close(m.schedulerStop)
			// Close queue? Or just wait for context cancel if we had one.
			// Currently worker reads from queue forever.
			// We can close queue here but ensure no writes happen after.
			// m.scan happens in this loop, so no new writes from scan.
			// But RetryObject might write.
			m.saveAllCaches(true)
			return
		}
	}
}

func (m *Manager) alignStorages() {
	tasks := make([]core.Task, 0, 64)
	m.tasks.Range(func(key, value any) bool {
		tasks = append(tasks, value.(core.Task))
		return true
	})
	for _, t := range tasks {
		if sp, ok := t.(core.StorageProvider); ok {
			st := sp.GetStorage()
			if st != nil {
				list, err := st.Search(nil)
				if err == nil {
					for _, obj := range list {
						if m.urlRegistry.Owners(obj.URL) < 2 {
							continue
						}
						m.urlRegistry.Update(obj)
						m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
					}
				}
			}
		}
	}
}

func (m *Manager) Stop() {
	// Ideally close mongo clients here too, but they are global in storage pkg currently
	close(m.stopChan)
}

func (m *Manager) scan() {
	// slog.Debug("Scanning tasks")

	if m.currentCfg().TaskScan.Disable {
		return
	}

	tasks := make([]core.Task, 0, 64)
	m.tasks.Range(func(key, value any) bool {
		tasks = append(tasks, value.(core.Task))
		return true
	})

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
		q := m.getTaskQueue(t.ID())
		select {
		case q <- &downloadRequest{task: t, obj: obj}:
			slog.Info("Object enqueued", "task_id", t.ID(), "url", obj.URL)

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

func (m *Manager) getTaskQueue(taskID string) chan *downloadRequest {
	if v, ok := m.taskQueues.Load(taskID); ok {
		return v.(chan *downloadRequest)
	}
	// size 32 per task queue
	q := make(chan *downloadRequest, 32)
	if v, loaded := m.taskQueues.LoadOrStore(taskID, q); loaded {
		return v.(chan *downloadRequest)
	}
	return q
}

func (m *Manager) scheduler() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	weights := make(map[string]int)
	lastUpdate := time.Now()
	for {
		select {
		case <-m.schedulerStop:
			return
		case <-ticker.C:
			if time.Since(lastUpdate) > 2*time.Second {
				weights = make(map[string]int)
				m.tasks.Range(func(key, value any) bool {
					id := key.(string)
					w := 1
					w += max(0, len(m.getTaskQueue(id))/8)
					if v, ok := m.metrics.Load(id); ok {
						mt := v.(*taskMetrics)
						if mt.avgLatencyMs > 5000 {
							w -= 1
						}
						if mt.failures > 0 {
							w -= min(mt.failures, 2)
						}
						if w < 1 {
							w = 1
						}
					}
					w = min(w, 8)
					weights[id] = w
					return true
				})
				lastUpdate = time.Now()
			}
			ids := make([]string, 0, 64)
			m.tasks.Range(func(key, value any) bool {
				ids = append(ids, key.(string))
				return true
			})
			expanded := make([]string, 0, 64)
			for _, id := range ids {
				w := weights[id]
				if w <= 0 {
					w = 1
				}
				for i := 0; i < w; i++ {
					expanded = append(expanded, id)
				}
			}
			for _, id := range expanded {
				q := m.getTaskQueue(id)
				select {
				case req := <-q:
					select {
					case m.downloadQueue <- req:
					default:
						// global queue full, put back
						go func(r *downloadRequest, tq chan *downloadRequest) {
							select {
							case tq <- r:
							default:
							}
						}(req, q)
						// break early to avoid tight loop
						break
					}
				default:
				}
			}
		}
	}
}

func (m *Manager) broadcastProgress() {
	m.downloadingObj.Range(func(key, value any) bool {
		obj := value.(*model.DownloadObject)

		// Check if progress has changed
		last, loaded := m.lastProgress.LoadOrStore(obj.URL, -1)
		if !loaded || last.(int) != obj.Progress {
			m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
			m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
			m.lastProgress.Store(obj.URL, obj.Progress)
		}
		return true
	})
}

func (m *Manager) BroadcastTaskUpdate(taskID string) {
	t, ok := m.getTask(taskID)

	if !ok {
		return
	}

	summary := map[string]any{
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
	{
		m.mu.Lock()
		summary["active"] = m.activeDownloads[taskID]
		m.mu.Unlock()
	}
	q := m.getTaskQueue(taskID)
	summary["backlog"] = len(q)
	if v, ok := m.metrics.Load(taskID); ok {
		mt := v.(*taskMetrics)
		summary["avg_latency_ms"] = mt.avgLatencyMs
		summary["failures"] = mt.failures
	}

	m.publish(core.Event{Type: core.EventTaskUpdate, Payload: summary})
}

func (m *Manager) getTask(id string) (core.Task, bool) {
	if v, ok := m.tasks.Load(id); ok {
		return v.(core.Task), true
	}
	return nil, false
}

func (m *Manager) download(t core.Task, obj *model.DownloadObject) {
	start := time.Now()
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
	m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})

	m.mu.Lock()
	dl := m.downloader
	m.mu.Unlock()

	err := dl.Download(obj, t.GetDownloadHeaders())
	if err != nil {
		if obj.Status == model.StatusCancelled {
			m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
			m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
			return
		}
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
		if v, ok := m.metrics.LoadOrStore(t.ID(), &taskMetrics{}); ok {
			mt := v.(*taskMetrics)
			mt.completed++
			elapsed := time.Since(start).Seconds() * 1000
			if mt.avgLatencyMs == 0 {
				mt.avgLatencyMs = elapsed
			} else {
				mt.avgLatencyMs = (mt.avgLatencyMs*0.7 + elapsed*0.3)
			}
		}
	}
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
	m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
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
func (m *Manager) GetActiveDownloads() []map[string]any {
	actives := make([]map[string]any, 0)
	m.downloadingObj.Range(func(key, value any) bool {
		obj := value.(*model.DownloadObject)
		actives = append(actives, map[string]any{
			"task_id":  obj.TaskID,
			"url":      obj.URL,
			"title":    obj.Metadata["title"],
			"progress": obj.Progress,
			"status":   obj.Status, // Should be 'downloading'
			"owners":   m.urlRegistry.Owners(obj.URL),
		})
		return true
	})
	return actives
}

func (m *Manager) GetTaskSummaries() []map[string]any {
	var summaries []map[string]any
	// Iterate using config order to maintain consistency
	for _, tCfg := range m.currentCfg().Tasks {
		id := tCfg.ID
		t, ok := m.getTask(id)
		if !ok {
			continue
		}

		summary := map[string]any{
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
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i]["id"].(string) < summaries[j]["id"].(string)
	})
	return summaries
}

func (m *Manager) GetTaskDetails(id string, page, limit int, search, sortBy string) (map[string]any, error) {
	t, ok := m.getTask(id)
	// also locate config entry for readonly fields
	var tCfg *config.Task
	cfg := m.currentCfg()
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == id {
			tCfg = &cfg.Tasks[i]
			break
		}
	}

	if !ok {
		return nil, fmt.Errorf("task not found")
	}

	result := map[string]any{
		"id":   t.ID(),
		"type": t.Type(),
	}
	if tCfg != nil {
		result["save_dir"] = tCfg.SaveDir
		result["storage"] = tCfg.Storage
		result["extra"] = tCfg.Extra
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
				} else if tags, ok := obj.Extra["tags"].([]any); ok {
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
			start := min((page-1)*limit, total)
			end := min(start+limit, total)
			pagedObjs = objs[start:end]
		}

		result["objects"] = pagedObjs
		result["total"] = total
		result["page"] = page
		result["limit"] = limit
	}

	return result, nil
}

func (m *Manager) AggregateObjects(page, limit int, search, sortBy, status string, types []string) (map[string]any, error) {
	all := make([]*model.DownloadObject, 0, 1024)
	cfg := m.currentCfg()
	typeMatches := func(t core.Task) bool {
		if len(types) == 0 {
			return true
		}
		tt := t.Type()
		for _, pref := range types {
			if strings.HasPrefix(tt, pref) {
				return true
			}
		}
		return false
	}
	for _, tCfg := range cfg.Tasks {
		id := tCfg.ID
		t, ok := m.getTask(id)
		if !ok {
			continue
		}
		if !typeMatches(t) {
			continue
		}
		if st, ok := t.(interface {
			GetAllObjects() []*model.DownloadObject
		}); ok {
			objs := st.GetAllObjects()
			for _, o := range objs {
				if status != "" && status != "all" {
					if o.Status != status {
						continue
					}
				}
				all = append(all, o)
			}
		}
	}
	if search = strings.ToLower(strings.TrimSpace(search)); search != "" {
		filtered := make([]*model.DownloadObject, 0, len(all))
		for _, obj := range all {
			match := false
			if strings.Contains(strings.ToLower(obj.URL), search) {
				match = true
			} else if title, ok := obj.Metadata["title"]; ok && strings.Contains(strings.ToLower(title), search) {
				match = true
			} else if tags, ok := obj.Extra["tags"].([]any); ok {
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
		all = filtered
	}
	sort.Slice(all, func(i, j int) bool {
		switch sortBy {
		case "date_asc":
			return all[i].Metadata["date"] < all[j].Metadata["date"]
		case "date_desc":
			return all[i].Metadata["date"] > all[j].Metadata["date"]
		case "name_asc":
			ti := all[i].Metadata["title"]
			if ti == "" {
				ti = all[i].URL
			}
			tj := all[j].Metadata["title"]
			if tj == "" {
				tj = all[j].URL
			}
			return strings.ToLower(ti) < strings.ToLower(tj)
		case "duration_desc":
			return all[i].Metadata["duration"] > all[j].Metadata["duration"]
		default:
			di := all[i].Metadata["date"]
			dj := all[j].Metadata["date"]
			if di != dj {
				return di > dj
			}
			return all[i].URL < all[j].URL
		}
	})
	total := len(all)
	var paged []*model.DownloadObject
	if limit <= 0 {
		paged = all
		page = 1
		limit = total
	} else {
		if page < 1 {
			page = 1
		}
		start := min((page-1)*limit, total)
		end := min(start+limit, total)
		paged = all[start:end]
	}
	return map[string]any{
		"objects": paged,
		"total":   total,
		"page":    page,
		"limit":   limit,
	}, nil
}

func hasMethod(i any, name string) bool {
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
	t, ok := m.getTask(taskID)

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
	t, ok := m.getTask(taskID)

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
	t, ok := m.getTask(taskID)

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

func (m *Manager) CancelTask(taskID string) error {
	t, ok := m.getTask(taskID)
	if !ok {
		return fmt.Errorf("task not found")
	}
	if st, ok := t.(interface {
		GetAllObjects() []*model.DownloadObject
	}); ok {
		objs := st.GetAllObjects()
		for _, obj := range objs {
			if obj.Status == model.StatusCompleted {
				continue
			}
			t.UpdateStatus(obj, model.StatusCancelled, nil)
			m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
			m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
			if _, active := m.downloadingObj.Load(obj.URL); active {
				if c, ok := m.downloader.(interface {
					Cancel(url string) error
				}); ok {
					_ = c.Cancel(obj.URL)
				}
				m.downloadingObj.Delete(obj.URL)
				m.mu.Lock()
				if m.activeDownloads[taskID] > 0 {
					m.activeDownloads[taskID]--
				}
				m.mu.Unlock()
			}
		}
		m.BroadcastTaskUpdate(taskID)
		return nil
	}
	return fmt.Errorf("task does not support object access")
}

func (m *Manager) CancelTasks(ids []string) map[string]string {
	result := make(map[string]string)
	for _, id := range ids {
		if err := m.CancelTask(id); err != nil {
			result[id] = err.Error()
		} else {
			result[id] = "ok"
		}
	}
	return result
}

// CancelObject 取消单个对象下载（对象级别）
func (m *Manager) CancelObject(taskID, url string) error {
	t, ok := m.getTask(taskID)
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
				t.UpdateStatus(obj, model.StatusCancelled, nil)
				m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
				m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
				if _, active := m.downloadingObj.Load(obj.URL); active {
					if c, ok := m.downloader.(interface {
						Cancel(url string) error
					}); ok {
						_ = c.Cancel(obj.URL)
					}
					m.downloadingObj.Delete(obj.URL)
					m.mu.Lock()
					if m.activeDownloads[taskID] > 0 {
						m.activeDownloads[taskID]--
					}
					m.mu.Unlock()
				}
				m.BroadcastTaskUpdate(taskID)
				return nil
			}
		}
		return fmt.Errorf("object not found")
	}
	return fmt.Errorf("task does not support object access")
}

// UndoCancelObject 撤销取消，将对象恢复为待下载
func (m *Manager) UndoCancelObject(taskID, url string) error {
	t, ok := m.getTask(taskID)
	if !ok {
		return fmt.Errorf("task not found")
	}
	if st, ok := t.(interface {
		GetAllObjects() []*model.DownloadObject
	}); ok {
		objs := st.GetAllObjects()
		for _, obj := range objs {
			if obj.URL == url {
				if obj.Status != model.StatusCancelled {
					return fmt.Errorf("object status is not cancelled")
				}
				t.UpdateStatus(obj, model.StatusPending, nil)
				obj.Progress = 0
				m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
				m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
				// 让调度器自然拾取，或立即触发该任务的处理
				go m.processTask(t)
				m.BroadcastTaskUpdate(taskID)
				return nil
			}
		}
		return fmt.Errorf("object not found")
	}
	return fmt.Errorf("task does not support object access")
}

// --- Config Management ---

func (m *Manager) UpdateConfig(newCfg *config.Config, audit *AuditInfo) error {
	// Validate before IO
	newCfg.ValidateAndClamp()
	// Save to file
	if err := m.writeConfigWithComments(newCfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	// Write backup and audit
	if name, err := m.writeConfigBackup(); err != nil {
		slog.Warn("Failed to write config backup", "error", err)
	} else if audit != nil {
		msg := audit.Message
		if msg == "" {
			msg = "config update"
		}
		if err := m.AddConfigNote(name, msg, audit.Author); err != nil {
			slog.Warn("Failed to add config note", "error", err, "filename", name, "message", msg)
		}
		if audit.Source != "" {
			if err := m.AddConfigTag(name, audit.Source); err != nil {
				slog.Warn("Failed to add config tag", "error", err, "filename", name, "tag", audit.Source)
			}
		}
	}
	// Apply in-memory config
	m.cfg = newCfg
	m.cfgVal.Store(newCfg)
	// Reload components
	m.downloader = downloader.New(newCfg.Downloader)
	logutil.InitLogger(newCfg.Log)
	// Runtime adjustments
	m.adjustGlobalWorkers(newCfg.Downloader.GlobalConcurrent)
	m.applyTaskRuntime(newCfg)
	// Load missing tasks
	m.loadTasks()
	// Notify
	slog.Info("Configuration updated")
	m.publish(core.Event{Type: core.EventTaskListChange, Payload: nil})
	go m.scan()
	return nil
}

func (m *Manager) UpdateLogConfig(newLog logutil.LogConfig) error {
	cur := m.GetConfig()
	cfgCopy := *cur
	cfgCopy.Log = newLog
	cfgCopy.ValidateAndClamp()
	if err := config.Save(config.GetConfigFilePath(), &cfgCopy); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	logutil.InitLogger(newLog)
	m.cfg = &cfgCopy
	m.cfgVal.Store(&cfgCopy)
	return nil
}
