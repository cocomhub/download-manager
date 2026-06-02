// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/downloader"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"
	"github.com/cocomhub/download-manager/pkg/logutil"
	"github.com/cocomhub/download-manager/pkg/scrape"
	"github.com/cocomhub/download-manager/pkg/titlegroup"
	"github.com/cocomhub/download-manager/storage"
	"github.com/cocomhub/download-manager/task/tktube"
)

type downloadRequest struct {
	task core.Task
	obj  *model.DownloadObject
}

type Manager struct {
	cfg           *config.Config
	cfgVal        atomic.Value
	configSvc     *ConfigService
	aggSvc        *AggregationService
	tasks         sync.Map
	downloader    core.Downloader
	stopChan      chan struct{}
	workerStop    chan struct{}
	workerCount   int
	taskQueues    sync.Map
	schedulerStop chan struct{}

	// Concurrency control
	activeDownloads map[string]int // TaskID -> Active Count (Just for stats/per-task limit if needed)
	mu              sync.Mutex
	downloadingObj  sync.Map // URL -> *model.DownloadObject (Active downloads)
	processingTask  sync.Map // TaskID -> bool (To track if task is being processed)
	scrapingTask    sync.Map // TaskID -> bool (To dedupe concurrent Scrape per task)
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

	// Scrape driver for full/incremental scan management
	scrapeDriver *scrape.Driver

	schedulerEnabled atomic.Bool
	workersEnabled   atomic.Bool
	scanRunning      atomic.Bool

	// Shutdown tracking for force-download goroutines
	forceWg sync.WaitGroup
}

type taskMetrics struct {
	avgLatencyMs atomic.Int64
	failures     atomic.Int64
	completed    atomic.Int64
}

type RuntimeFeatures struct {
	Scheduler bool `json:"scheduler"`
	Workers   bool `json:"workers"`
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
		configSvc:       NewConfigService(cfg),
		aggSvc:          NewAggregationService(nil, nil, nil, nil),
		downloader:      downloader.New(cfg.Downloader),
		stopChan:        make(chan struct{}),
		workerStop:      make(chan struct{}, 256),
		activeDownloads: make(map[string]int),
		downloadQueue:   make(chan *downloadRequest, max(globalLimit*2, 10)), // Buffer size
		subscribers:     make(map[<-chan core.Event]chan core.Event),
		urlRegistry:     NewURLStateRegistry(),
	}
	mgr.cfgVal.Store(cfg)
	tracker := scrape.NewFileTracker(filepath.Join(cfg.Server.WorkDir, "cache", "task"))
	mgr.scrapeDriver = scrape.NewDriver(tracker, scrape.NewDefaultPager())
	if nd, ok := mgr.downloader.(*downloader.NativeHTTPDownloader); ok {
		nd.ApplyDomainLimits(cfg.Downloader.DomainLimits)
	}
	// Wire up AggregationService with real callbacks
	mgr.aggSvc = NewAggregationService(
		mgr.getAllTasks,
		mgr.searchTaskObjects,
		mgr.countTaskObjects,
		mgr.collectTaskObjects,
	)
	return mgr
}

func (m *Manager) FeaturesStatus() RuntimeFeatures {
	return RuntimeFeatures{Scheduler: m.schedulerEnabled.Load(), Workers: m.workersEnabled.Load()}
}

// getAllTasks returns all registered tasks as a flat slice.
func (m *Manager) getAllTasks() []core.Task {
	var tasks []core.Task
	m.tasks.Range(func(_, value any) bool {
		tasks = append(tasks, value.(core.Task))
		return true
	})
	return tasks
}

func (m *Manager) GetDownloadRootDir() string {
	cfg := m.currentCfg()
	if cfg != nil && cfg.Server.DownloadRootDir != "" {
		return cfg.Server.DownloadRootDir
	}
	// Fallback for test / nil config
	wd := config.GetWorkDir()
	return filepath.Join(wd, "downloads")
}

func (m *Manager) Subscribe() <-chan core.Event {
	m.eventMu.Lock()
	defer m.eventMu.Unlock()
	ch := make(chan core.Event, 100) // Buffer to prevent blocking
	m.subscribers[ch] = ch
	return ch
}

func (m *Manager) currentCfg() *config.Config {
	return m.configSvc.GetConfig()
}

func cloneStorageQuery(query *core.StorageQuery) *core.StorageQuery {
	if query == nil {
		return &core.StorageQuery{}
	}
	cloned := *query
	cloned.Filter.TaskIDs = append([]string(nil), query.Filter.TaskIDs...)
	cloned.Filter.URLs = append([]string(nil), query.Filter.URLs...)
	cloned.Filter.Statuses = append([]string(nil), query.Filter.Statuses...)
	if query.Filter.Metadata != nil {
		cloned.Filter.Metadata = make(map[string]string, len(query.Filter.Metadata))
		maps.Copy(cloned.Filter.Metadata, query.Filter.Metadata)
	}
	cloned.Sort = append([]core.StorageSort(nil), query.Sort...)
	return &cloned
}

func queryForTask(taskID string, query *core.StorageQuery) *core.StorageQuery {
	cloned := cloneStorageQuery(query)
	cloned.Filter.TaskIDs = []string{strings.TrimSpace(taskID)}
	return cloned
}

func sortRules(sortBy string) []core.StorageSort {
	switch sortBy {
	case "date_asc":
		return []core.StorageSort{{Field: "date"}, {Field: "url"}}
	case "date_desc":
		return []core.StorageSort{{Field: "date", Desc: true}, {Field: "url"}}
	case "name_asc":
		return []core.StorageSort{{Field: "name"}, {Field: "url"}}
	case "duration_desc":
		return []core.StorageSort{{Field: "duration", Desc: true}, {Field: "url"}}
	default:
		return []core.StorageSort{{Field: "date", Desc: true}, {Field: "url"}}
	}
}

func (m *Manager) searchTaskObjects(t core.Task, query *core.StorageQuery) ([]*model.DownloadObject, error) {
	taskQuery := queryForTask(t.ID(), query)
	if st := t.Storage(); st != nil {
		return st.Search(taskQuery)
	}
	if accessor, ok := t.(interface {
		GetAllObjects(lock bool) []*model.DownloadObject
	}); ok {
		return storage.ApplyQueryToObjects(accessor.GetAllObjects(true), taskQuery), nil
	}
	return []*model.DownloadObject{}, nil
}

func (m *Manager) countTaskObjects(t core.Task, query *core.StorageQuery) (int64, error) {
	taskQuery := queryForTask(t.ID(), query)
	if st := t.Storage(); st != nil {
		return st.Count(taskQuery)
	}
	if accessor, ok := t.(interface {
		GetAllObjects(lock bool) []*model.DownloadObject
	}); ok {
		return storage.CountObjects(accessor.GetAllObjects(true), taskQuery), nil
	}
	return 0, nil
}

func (m *Manager) collectTaskObjects(t core.Task, query *core.StorageQuery, batchSize int64) ([]*model.DownloadObject, error) {
	if query != nil && query.Limit > 0 {
		return m.searchTaskObjects(t, query)
	}
	if batchSize <= 0 {
		batchSize = 200
	}
	collected := make([]*model.DownloadObject, 0, batchSize)
	var offset int64
	for {
		pageQuery := cloneStorageQuery(query)
		pageQuery.Offset = offset
		pageQuery.Limit = batchSize
		chunk, err := m.searchTaskObjects(t, pageQuery)
		if err != nil {
			return nil, err
		}
		if len(chunk) == 0 {
			break
		}
		collected = append(collected, chunk...)
		if int64(len(chunk)) < batchSize {
			break
		}
		offset += int64(len(chunk))
	}
	return collected, nil
}

func (m *Manager) getTaskObject(t core.Task, url string) (*model.DownloadObject, error) {
	list, err := m.searchTaskObjects(t, &core.StorageQuery{
		Filter: core.StorageFilter{
			URLs: []string{url},
		},
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
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
	cfg := m.currentCfg()
	slog.Info("runtime mode", "mode", cfg.Runtime.Mode, "download", cfg.Runtime.Download.Enabled, "scheduler", cfg.Runtime.Scheduler.Enabled)
	m.workersEnabled.Store(cfg.Runtime.Mode != config.RunModeUI && cfg.Runtime.Download.Enabled)
	m.schedulerEnabled.Store(cfg.Runtime.Mode != config.RunModeUI && cfg.Runtime.Scheduler.Enabled)
	slog.Info("disabled components", "scheduler", !m.schedulerEnabled.Load(), "workers", !m.workersEnabled.Load())
	m.loadTasks()
	if m.workersEnabled.Load() {
		limit := m.currentCfg().Downloader.GlobalConcurrent
		if limit <= 0 {
			limit = 5
		}
		slog.Info("Starting global workers", "count", limit)
		for i := 0; i < limit; i++ {
			m.workerWg.Add(1)
			go m.worker()
		}
		m.workerCount = limit
	}
	if m.schedulerEnabled.Load() {
		m.schedulerStop = make(chan struct{})
		go m.scheduler()
	}

	interval := time.Duration(m.currentCfg().TaskScan.Interval) * time.Second
	if interval == 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)

	// Progress broadcast ticker
	progressTicker := time.NewTicker(1 * time.Second)

	defer ticker.Stop()
	defer progressTicker.Stop()

	// Immediate scan on start
	m.scan()

	for {
		select {
		case <-ticker.C:
			m.scan()
		case <-progressTicker.C:
			m.broadcastProgress()
		case <-m.stopChan:
			slog.Info("Manager stopping")
			if m.schedulerStop != nil {
				close(m.schedulerStop)
			}
			m.closeAllTasks()
			return
		}
	}
}

func (m *Manager) Stop(ctx context.Context) {
	slog.Info("Manager stopping")

	// 1. Signal workers to stop first — no new downloads
	close(m.stopChan)

	// 2. Wait for workers and force-downloads with context deadline
	done := make(chan struct{})
	go func() {
		m.workerWg.Wait()
		m.forceWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("All workers stopped")
	case <-ctx.Done():
		slog.Warn("Shutdown timed out, some workers may still be running")
	}

	// 3. Mark survivors (e.g. force-download goroutines that didn't finish) as failed
	m.downloadingObj.Range(func(key, value any) bool {
		obj := value.(*model.DownloadObject)
		if t, ok := m.getTask(obj.TaskID); ok {
			t.UpdateStatus(obj, dlcore.StatusFailed, errors.New("shutdown"))
			m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
			m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
		}
		return true
	})
}

// WaitForShutdown waits for workers and force-downloads to finish, then flushes storages.
// It respects the provided context deadline.
func (m *Manager) WaitForShutdown(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		m.workerWg.Wait()
		m.forceWg.Wait()
		m.flushAllStorages()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("All workers stopped and storages flushed")
	case <-ctx.Done():
		slog.Warn("Shutdown timed out, some workers may still be running")
	}
}

func (m *Manager) flushAllStorages() {
	m.tasks.Range(func(key, value any) bool {
		t := value.(core.Task)
		if flusher, ok := t.Storage().(interface{ ForceFlush() error }); ok {
			if err := flusher.ForceFlush(); err != nil {
				slog.Error("Failed to flush storage", "task_id", t.ID(), "error", err)
			}
		}
		return true
	})
	slog.Info("All storages flushed")
}

func (m *Manager) scan() {
	// slog.Debug("Scanning tasks")
	if !m.workersEnabled.Load() {
		return
	}

	if m.currentCfg().TaskScan.Disable {
		return
	}

	if !m.scanRunning.CompareAndSwap(false, true) {
		slog.Debug("scan: already running, skipping")
		return
	}
	defer m.scanRunning.Store(false)

	// Phase 1: Scrape — discover new objects from tasks that support it.
	// Run scrapes in detached goroutines with per-task ctx timeout and per-task
	// dedup guard (scrapingTask) so a slow Scrape never overlaps itself.
	// Do NOT wait — Phase 2 runs in parallel; scraped objects are persisted
	// to storage and picked up by the next scan cycle's Phase 2.
	m.tasks.Range(func(key, value any) bool {
		if sc, ok := value.(core.ScrapeCap); ok {
			taskID := key.(string)
			if _, scraping := m.scrapingTask.LoadOrStore(taskID, true); scraping {
				slog.Debug("Scrape: previous run still in progress, skipping", "task_id", taskID)
				return true
			}
			go func(taskID string, sc core.ScrapeCap) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				done := make(chan error, 1)
				go func() {
					done <- sc.Scrape(ctx)
				}()
				select {
				case err := <-done:
					if err != nil {
						slog.Error("Scrape failed", "task_id", taskID, "error", err)
					}
				case <-ctx.Done():
					slog.Error("Scrape timed out", "task_id", taskID)
					// ctx is canceled; wait for inner goroutine to actually return
					// before releasing the dedup guard, so the next scan cycle
					// does not start a second concurrent Scrape for this task.
					<-done
				}
				m.scrapingTask.Delete(taskID)
			}(taskID, sc)
		}
		return true
	})

	// Phase 2: Download — process tasks for pending objects
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

	limit := t.Concurrency()

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
						if mt.avgLatencyMs.Load() > 5000 {
							w -= 1
						}
						if mt.failures.Load() > 0 {
							w -= int(min(mt.failures.Load(), int64(2)))
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
		outerLoop:
			for _, id := range expanded {
				q := m.getTaskQueue(id)
				select {
				case req := <-q:
					select {
					case m.downloadQueue <- req:
					default:
						// global queue full, put back
						select {
						case q <- req:
						default:
							// task queue also full, drop -- next scan() will re-enqueue
						}
						break outerLoop
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
		if !loaded || last.(int) != obj.GetProgress() {
			m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
			m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
			m.lastProgress.Store(obj.URL, obj.GetProgress())
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

	if total, err := m.countTaskObjects(t, nil); err == nil {
		summary["total"] = total
	}
	if completed, err := m.countTaskObjects(t, &core.StorageQuery{
		Filter: core.StorageFilter{
			Statuses: []string{dlcore.StatusCompleted},
		},
	}); err == nil {
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
		summary["avg_latency_ms"] = mt.avgLatencyMs.Load()
		summary["failures"] = mt.failures.Load()
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

	// Check if manager is stopping — avoids overwriting status set by Stop()
	select {
	case <-m.stopChan:
		slog.Info("Download skipped — manager stopping", "url", obj.URL)
		return
	default:
	}

	t.UpdateStatus(obj, dlcore.StatusDownloading, nil)
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
	m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})

	m.mu.Lock()
	dl := m.downloader
	m.mu.Unlock()

	// Create per-download context tied to manager lifecycle for cancellation
	dlCtx, dlCancel := context.WithCancel(context.Background())
	defer dlCancel()
	go func() {
		select {
		case <-m.stopChan:
			dlCancel()
		case <-dlCtx.Done():
		}
	}()

	// Propagate context to NativeHTTPDownloader if supported
	if nd, ok := dl.(*downloader.NativeHTTPDownloader); ok {
		nd.SetContext(dlCtx)
	}

	err := dl.Download(obj, t.GetDownloadHeaders())
	if err != nil {
		if obj.GetStatus() == dlcore.StatusCancelled {
			m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
			m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
			return
		}
		slog.Error("Download failed", "task_id", t.ID(), "url", obj.URL, "error", err)
		t.UpdateStatus(obj, dlcore.StatusFailed, err)

		if dlcore.IsNoTry(err) {
			if ft, ok := t.(core.FailedTask); ok {
				ft.MarkAsFailed(obj, err)
			}
		}

		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return
		}

		// Increment failed count
		v, _ := m.failedCount.LoadOrStore(obj.URL, new(atomic.Int64))
		c := v.(*atomic.Int64).Add(1)
		// Check if max retries reached
		if c >= 5 {
			if ft, ok := t.(core.FailedTask); ok {
				ft.MarkAsFailed(obj, fmt.Errorf("max retries reached: %w", err))
			}
		}
	} else {
		t.UpdateStatus(obj, dlcore.StatusCompleted, nil)
		// Reset failed count on success
		m.failedCount.Delete(obj.URL)
		// Apply group priority policies for content groups
		m.applyGroupPriorityPolicies(t, obj)
		if v, ok := m.metrics.LoadOrStore(t.ID(), &taskMetrics{}); ok {
			mt := v.(*taskMetrics)
			mt.completed.Add(1)
			elapsed := time.Since(start).Seconds() * 1000
			if mt.avgLatencyMs.Load() == 0 {
				mt.avgLatencyMs.Store(int64(elapsed))
			} else {
				for {
					old := mt.avgLatencyMs.Load()
					newVal := int64(float64(old)*0.7 + elapsed*0.3)
					if mt.avgLatencyMs.CompareAndSwap(old, newVal) {
						break
					}
				}
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
	m.forceWg.Go(func() {
		m.download(t, obj)
	})
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
			"progress": obj.GetProgress(),
			"status":   obj.GetStatus(), // Should be 'downloading'
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

		if total, err := m.countTaskObjects(t, nil); err == nil {
			summary["total"] = total
		}
		if completed, err := m.countTaskObjects(t, &core.StorageQuery{
			Filter: core.StorageFilter{
				Statuses: []string{dlcore.StatusCompleted},
			},
		}); err == nil {
			summary["completed"] = completed
		}

		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i]["id"].(string) < summaries[j]["id"].(string)
	})
	return summaries
}

func (m *Manager) GetTaskDetails(id string, page, limit int64, search, sortBy string) (map[string]any, error) {
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
	result["concurrency"] = t.Concurrency()
	result["refresh_interval"] = t.RefreshInterval()
	result["supports"] = map[string]bool{
		"concurrency":      true,
		"refresh_interval": true,
	}

	if page < 1 {
		page = 1
	}
	var offset int64
	if limit > 0 {
		offset = (page - 1) * limit
	} else {
		page = 1
	}
	baseQuery := &core.StorageQuery{
		Filter: core.StorageFilter{
			Search: search,
		},
		Sort:   sortRules(sortBy),
		Offset: offset,
		Limit:  limit,
	}
	total, err := m.countTaskObjects(t, &core.StorageQuery{
		Filter: core.StorageFilter{
			Search: search,
		},
	})
	if err != nil {
		return nil, err
	}
	var objs []*model.DownloadObject
	if limit > 0 {
		objs, err = m.searchTaskObjects(t, baseQuery)
	} else {
		objs, err = m.collectTaskObjects(t, baseQuery, 200)
	}
	if err != nil {
		return nil, err
	}
	if objs == nil {
		objs = make([]*model.DownloadObject, 0)
	}
	if limit <= 0 {
		limit = total
	}
	result["objects"] = objs
	result["total"] = total
	result["page"] = page
	result["limit"] = limit

	return result, nil
}

func (m *Manager) AggregateObjects(page, limit int64, search, sortBy, status string, types []string) (map[string]any, error) {
	return m.aggSvc.AggregateObjects(page, limit, search, sortBy, status, types)
}

// AggregateByContent groups objects by scoped content group and returns representatives.
func (m *Manager) AggregateByContent(page, limit int64, search, sortBy, status string, types []string) (map[string]any, error) {
	cfg := m.currentCfg()
	typeMatches := func(t core.Task) bool {
		if len(types) == 0 {
			return true
		}
		tt := strings.ToLower(t.Type())
		for _, pref := range types {
			p := strings.ToLower(pref)
			if strings.HasPrefix(tt, p) {
				return true
			}
		}
		return false
	}
	type taskObj struct {
		t   core.Task
		obj *model.DownloadObject
	}

	// Collect matching tasks
	var matchingTasks []core.Task
	for _, tCfg := range cfg.Tasks {
		id := tCfg.ID
		tk, ok := m.getTask(id)
		if !ok {
			continue
		}
		if !typeMatches(tk) {
			continue
		}
		matchingTasks = append(matchingTasks, tk)
	}

	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 50
	}

	// For content grouping we need ALL matching objects to build groups properly,
	// but we collect each task via Search with search/status filter to reduce data.
	all := make([]taskObj, 0, 1024)
	for _, tk := range matchingTasks {
		query := &core.StorageQuery{
			Filter: core.StorageFilter{
				Search: search,
			},
		}
		if status != "" && status != "all" {
			query.Filter.Statuses = []string{status}
		}
		objs, err := m.collectTaskObjects(tk, query, 200)
		if err != nil {
			return nil, err
		}
		for _, o := range objs {
			all = append(all, taskObj{t: tk, obj: o})
		}
	}
	// Group by task_id + task_type + content_group to avoid cross-task leakage.
	type groupEntry struct {
		t   core.Task
		obj *model.DownloadObject
	}
	groups := make(map[string][]groupEntry)
	for _, to := range all {
		key := scopedContentGroupKey(to.t.ID(), to.t.Type(), metadataContentGroup(to.obj))
		groups[key] = append(groups[key], groupEntry(to))
	}
	// Pick representative by priority, tie -> first.
	reps := make([]*model.DownloadObject, 0, len(groups))
	for _, entries := range groups {
		var rep *model.DownloadObject
		repScore := -1
		for idx, e := range entries {
			score := variantPriorityScore(e.t, e.obj)
			if idx == 0 || score > repScore {
				rep = e.obj
				repScore = score
			}
		}
		if rep != nil {
			// shallow copy Extra/Metadata without copying mu
			copyObj := &model.DownloadObject{
				TaskID:   rep.TaskID,
				URL:      rep.URL,
				SavePath: rep.SavePath,
				Status:   rep.GetStatus(),
				Progress: rep.GetProgress(),
			}
			if rep.Metadata != nil {
				copyObj.Metadata = make(map[string]string, len(rep.Metadata))
				maps.Copy(copyObj.Metadata, rep.Metadata)
			}
			copyObj.Extra = make(map[string]any, len(rep.Extra)+1)
			if rep.Extra != nil {
				maps.Copy(copyObj.Extra, rep.Extra)
			}
			copyObj.Extra["group_size"] = len(entries)
			reps = append(reps, copyObj)
		}
	}
	total := int64(len(reps))
	if page < 1 {
		page = 1
	}
	var offset int64
	if limit <= 0 {
		page = 1
		limit = total
	} else {
		offset = (page - 1) * limit
	}
	paged := storage.ApplyQueryToObjects(reps, &core.StorageQuery{
		Sort:   sortRules(sortBy),
		Offset: offset,
		Limit:  limit,
	})
	return map[string]any{
		"objects": paged,
		"total":   total,
		"page":    page,
		"limit":   limit,
	}, nil
}

func metadataContentGroup(obj *model.DownloadObject) string {
	if obj == nil || obj.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(obj.Metadata["content_group"])
}

func metadataTaskType(obj *model.DownloadObject) string {
	if obj == nil || obj.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(obj.Metadata["task_type"])
}

func scopedContentGroupKey(taskID, taskType, group string) string {
	return strings.TrimSpace(taskID) + "\x00" + strings.TrimSpace(taskType) + "\x00" + strings.TrimSpace(group)
}

func variantPriorityScore(t core.Task, obj *model.DownloadObject) int {
	if t == nil || obj == nil || t.Type() != tktube.TaskType {
		return 0
	}
	hq, c := titlegroup.TKTVariantFlags(obj.Metadata["title"])
	switch {
	case hq && c:
		return 4
	case hq:
		return 3
	case c:
		return 2
	default:
		return 1
	}
}

// BackfillContentGroups scans storages and recomputes content_group/task_type metadata for tktube tasks.
func (m *Manager) BackfillContentGroups() {
	m.tasks.Range(func(key, value any) bool {
		t, _ := value.(core.Task)
		if t == nil || t.Type() != tktube.TaskType {
			return true
		}
		st := t.Storage()
		if st == nil {
			return true
		}
		list, err := m.collectTaskObjects(t, &core.StorageQuery{
			Filter: core.StorageFilter{
				TaskIDs: []string{strings.TrimSpace(t.ID())},
			},
		}, 200)
		if err != nil || list == nil {
			return true
		}
		total := 0
		changed := 0
		taskType := strings.TrimSpace(t.Type())
		for _, obj := range list {
			if obj == nil {
				continue
			}
			total++
			if obj.Metadata == nil {
				obj.Metadata = make(map[string]string)
			}
			dirty := false
			newGroup := titlegroup.TKTContentGroupKey(obj.Metadata["title"], obj.URL)
			if obj.Metadata["content_group"] != newGroup {
				obj.Metadata["content_group"] = newGroup
				dirty = true
			}
			if obj.Metadata["task_type"] != taskType {
				obj.Metadata["task_type"] = taskType
				dirty = true
			}
			if !dirty {
				continue
			}
			if err := st.Update(obj); err != nil {
				slog.Warn("Failed to recompute object metadata", "task_id", t.ID(), "url", obj.URL, "error", err)
				continue
			}
			changed++
		}
		slog.Info("Recomputed object metadata", "task_id", t.ID(), "task_type", t.Type(), "total", total, "changed", changed)
		return true
	})
}

// applyGroupPriorityPolicies enforces group priority within the current tktube task only.
// Even if multiple tasks share the same storage, only objects whose TaskID matches t.ID()
// and whose task_type/content_group match the completed object are eligible.
func (m *Manager) applyGroupPriorityPolicies(t core.Task, obj *model.DownloadObject) {
	if t.Type() != tktube.TaskType {
		return
	}
	if obj == nil || obj.GetStatus() != dlcore.StatusCompleted {
		return
	}
	taskType := strings.TrimSpace(t.Type())
	if taskType == "" || metadataTaskType(obj) != taskType {
		return
	}
	group := metadataContentGroup(obj)
	if strings.TrimSpace(group) == "" {
		return
	}
	taskID := strings.TrimSpace(t.ID())
	if taskID == "" || strings.TrimSpace(obj.TaskID) != taskID {
		return
	}
	st := t.Storage()
	if st == nil {
		return
	}
	list, err := m.collectTaskObjects(t, &core.StorageQuery{
		Filter: core.StorageFilter{
			Metadata: map[string]string{"task_type": taskType, "content_group": group},
		},
	}, 200)
	if err != nil || list == nil {
		return
	}
	type candidate struct {
		o     *model.DownloadObject
		score int
	}
	var canonical *model.DownloadObject
	bestScore := -1
	cands := make([]candidate, 0, 8)
	priorityCounts := make(map[int]int, 4)
	for _, o := range list {
		if o == nil {
			continue
		}
		if strings.TrimSpace(o.TaskID) != taskID {
			continue
		}
		if metadataTaskType(o) != taskType {
			continue
		}
		if metadataContentGroup(o) != group {
			continue
		}
		score := variantPriorityScore(t, o)
		cands = append(cands, candidate{o: o, score: score})
		priorityCounts[score]++
		if o.GetStatus() == dlcore.StatusCompleted {
			if canonical == nil || score > bestScore {
				canonical = o
				bestScore = score
			}
		}
	}
	for priority, count := range priorityCounts {
		if count > 1 {
			slog.Info("Skip auto-cancel for conflicting content group priority", "task_id", t.ID(), "task_type", t.Type(), "content_group", group, "priority", priority, "count", count)
			return
		}
	}
	if canonical == nil {
		return
	}
	for _, cnd := range cands {
		o := cnd.o
		if o.URL == canonical.URL {
			continue
		}
		// Auto-cancel only lower-priority pending objects.
		if cnd.score < bestScore && o.GetStatus() == dlcore.StatusPending {
			if o.Extra == nil {
				o.Extra = make(map[string]any)
			}
			o.Extra["redirect_url"] = canonical.URL
			if err := t.UpdateStatus(o, dlcore.StatusCancelled, nil); err != nil {
				slog.Warn("Failed to auto-cancel lower-priority duplicate", "task_id", t.ID(), "url", o.URL, "error", err)
			}
		}
	}
}

// GetObjectsByScopedGroup returns all objects for the given task_id + task_type + content_group.
func (m *Manager) GetObjectsByScopedGroup(taskID, taskType, group string) []*model.DownloadObject {
	list := make([]*model.DownloadObject, 0, 64)
	taskID = strings.TrimSpace(taskID)
	taskType = strings.TrimSpace(taskType)
	group = strings.TrimSpace(group)
	tk, ok := m.getTask(taskID)
	if !ok || tk.Type() != taskType {
		return list
	}
	objs, err := m.collectTaskObjects(tk, &core.StorageQuery{
		Filter: core.StorageFilter{
			Metadata: map[string]string{"content_group": group},
		},
	}, 200)
	if err == nil {
		list = append(list, objs...)
	}
	return list
}

// RetryObject resets the status of an object to pending and forces download
func (m *Manager) RetryObject(taskID, url string) error {
	t, ok := m.getTask(taskID)

	if !ok {
		return fmt.Errorf("task not found")
	}

	obj, err := m.getTaskObject(t, url)
	if err != nil {
		return err
	}
	if obj != nil {
		if obj.GetStatus() == dlcore.StatusCompleted {
			return fmt.Errorf("object already completed")
		}
		// Reset status
		t.UpdateStatus(obj, dlcore.StatusPending, nil)
		obj.SetProgress(0)

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
	return fmt.Errorf("object not found")
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

	objs, err := m.collectTaskObjects(t, &core.StorageQuery{
		Filter: core.StorageFilter{
			Statuses: []string{dlcore.StatusFailed, dlcore.StatusFailedPermanent},
		},
	}, 200)
	if err != nil {
		return err
	}
	count := 0
	for _, obj := range objs {
		t.UpdateStatus(obj, dlcore.StatusPending, nil)
		obj.SetProgress(0)
		count++
	}
	if count > 0 {
		go m.processTask(t)
	}
	return nil
}

func (m *Manager) CancelTask(taskID string) error {
	t, ok := m.getTask(taskID)
	if !ok {
		return fmt.Errorf("task not found")
	}
	objs, err := m.collectTaskObjects(t, &core.StorageQuery{}, 200)
	if err != nil {
		return err
	}
	for _, obj := range objs {
		if obj.GetStatus() == dlcore.StatusCompleted {
			continue
		}
		t.UpdateStatus(obj, dlcore.StatusCancelled, nil)
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
	obj, err := m.getTaskObject(t, url)
	if err != nil {
		return err
	}
	if obj == nil {
		return fmt.Errorf("object not found")
	}
	if obj.GetStatus() == dlcore.StatusCompleted {
		return fmt.Errorf("object already completed")
	}
	t.UpdateStatus(obj, dlcore.StatusCancelled, nil)
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

// UndoCancelObject 撤销取消，将对象恢复为待下载
func (m *Manager) UndoCancelObject(taskID, url string) error {
	t, ok := m.getTask(taskID)
	if !ok {
		return fmt.Errorf("task not found")
	}
	obj, err := m.getTaskObject(t, url)
	if err != nil {
		return err
	}
	if obj == nil {
		return fmt.Errorf("object not found")
	}
	if obj.GetStatus() != dlcore.StatusCancelled {
		return fmt.Errorf("object status is not cancelled")
	}
	t.UpdateStatus(obj, dlcore.StatusPending, nil)
	obj.SetProgress(0)
	m.publish(core.Event{Type: core.EventObjectUpdate, Payload: obj})
	m.publish(core.Event{Type: core.EventSharedObjectUpdate, Payload: obj})
	go m.processTask(t)
	m.BroadcastTaskUpdate(taskID)
	return nil
}

// --- Config Management ---

func (m *Manager) UpdateConfig(newCfg *config.Config, audit *AuditInfo) error {
	// Validate before IO
	newCfg.ValidateAndClamp()
	// Save to file with comment preservation
	if err := m.configSvc.WriteConfigWithComments(newCfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	// Write backup and audit
	if name, err := m.configSvc.writeConfigBackup(); err != nil {
		slog.Warn("Failed to write config backup", "error", err)
	} else if audit != nil {
		msg := audit.Message
		if msg == "" {
			msg = "config update"
		}
		if err := m.configSvc.AddConfigNote(name, msg, audit.Author); err != nil {
			slog.Warn("Failed to add config note", "error", err, "filename", name, "message", msg)
		}
		if audit.Source != "" {
			if err := m.configSvc.AddConfigTag(name, audit.Source); err != nil {
				slog.Warn("Failed to add config tag", "error", err, "filename", name, "tag", audit.Source)
			}
		}
	}
	// Apply in-memory config
	m.configSvc.StoreConfig(newCfg)
	// Reload components
	m.downloader = downloader.New(newCfg.Downloader)
	logutil.InitLogger(newCfg.Log)
	// Runtime adjustments
	m.adjustGlobalWorkers(newCfg.Downloader.GlobalConcurrent)
	m.applyTaskRuntime(newCfg)

	// Reconcile scheduler runtime state
	cfg := m.currentCfg()
	schedulerWanted := cfg.Runtime.Mode != config.RunModeUI && cfg.Runtime.Scheduler.Enabled
	if schedulerWanted && !m.schedulerEnabled.Load() {
		m.schedulerEnabled.Store(true)
		m.schedulerStop = make(chan struct{})
		go m.scheduler()
		slog.Info("Scheduler started via config update")
	} else if !schedulerWanted && m.schedulerEnabled.Load() {
		if m.schedulerStop != nil {
			close(m.schedulerStop)
		}
		m.schedulerEnabled.Store(false)
		slog.Info("Scheduler stopped via config update")
	}

	// Reconcile worker runtime state
	workersWanted := cfg.Runtime.Mode != config.RunModeUI && cfg.Runtime.Download.Enabled
	if workersWanted && !m.workersEnabled.Load() {
		m.workersEnabled.Store(true)
		m.adjustGlobalWorkers(cfg.Downloader.GlobalConcurrent)
		slog.Info("Workers enabled via config update")
	} else if !workersWanted && m.workersEnabled.Load() {
		m.workersEnabled.Store(false)
		for i := 0; i < m.workerCount; i++ {
			m.workerStop <- struct{}{}
		}
		slog.Info("Workers disabled via config update")
	}

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
	m.configSvc.StoreConfig(&cfgCopy)
	return nil
}
