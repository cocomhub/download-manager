// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

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

// New API methods

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
