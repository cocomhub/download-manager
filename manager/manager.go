// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
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
	"github.com/cocomhub/download-manager/pkg/logutil"
	"github.com/cocomhub/download-manager/pkg/scrape"
	"github.com/cocomhub/download-manager/storage"
)

// FailureRecord 描述单次下载失败记录，存储在环形缓冲区中供查询
type FailureRecord struct {
	TaskID    string `json:"task_id"`
	URL       string `json:"url"`
	Error     string `json:"error"`
	Attempt   int    `json:"attempt"`
	Timestamp int64  `json:"timestamp"`
	Permanent bool   `json:"permanent"`
}

type downloadRequest struct {
	task core.Task
	obj  *model.DownloadObject
}

type Manager struct {
	cfg             *config.Config
	cfgVal          atomic.Value
	configSvc       *ConfigService
	aggSvc          *AggregationService
	tasks           sync.Map
	downloader      core.Downloader
	downloaderMu    sync.Mutex
	stopChan        chan struct{}
	workerStop      chan struct{}
	workerCount     atomic.Int64
	taskQueues      sync.Map
	schedulerStop   chan struct{}
	schedulerSignal chan struct{} // buffered(1): enqueue -> wake scheduler. Fixed channel, initialized once in NewManager, not rebuilt on restart.

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

	// Heartbeat / uptime
	startedAt          time.Time    // 进程启动时间, set in NewManager
	totalDownloads     atomic.Int64 // 历史总下载次数
	schedulerHeartbeat atomic.Value // time.Time — 调度器最后心跳
	workerHeartbeat    atomic.Value // time.Time — worker 最后心跳

	// Failure records (ring buffer)
	failureRecords  []FailureRecord
	failureMu       sync.Mutex
	failureWriteIdx int // 环形缓冲区写入索引
	maxFailures     int // 环形容量（默认 1000）

	// Composite resolve retry tracking
	compositeResolveCount sync.Map // URL -> *atomic.Int64 (retry count, max 10)

	// Async resolve pool
	resolveCache  *ResolveCache
	resolveQueue  chan resolveRequest
	resolveCtx    context.Context
	resolveCancel context.CancelFunc
	resolveWg     sync.WaitGroup

	// Small-object pool
	soQueue    chan smallObjectRequest
	soCtx      context.Context
	soCancel   context.CancelFunc
	soWg       sync.WaitGroup
	soTracker  sync.Map // map[objKey]*objectTracker
	soInflight sync.Map // key: taskID+"\x00"+SavePath（SavePath 为空则用 URL）-> struct{}；在途小对象去重
	soPending  sync.Map // key: smallObjectKey -> pendingSO；队列满被丢弃的小对象，待补下载

	initializedCh chan struct{} // closed when Start() initialization completes
}

type taskMetrics struct {
	avgLatencyMs atomic.Int64
	failures     atomic.Int64
	completed    atomic.Int64
	// 新增
	retried    atomic.Int64 // 重试次数
	lastActive atomic.Int64 // 最近活跃 unix 秒
}

type RuntimeFeatures struct {
	Scheduler bool `json:"scheduler"`
	Workers   bool `json:"workers"`
}

// getDownloader returns the current downloader under read lock.
func (m *Manager) getDownloader() core.Downloader {
	m.downloaderMu.Lock()
	defer m.downloaderMu.Unlock()
	return m.downloader
}

// setDownloader replaces the downloader under write lock.
func (m *Manager) setDownloader(dl core.Downloader) {
	m.downloaderMu.Lock()
	// 关闭旧 Transport 的空闲连接
	if old, ok := m.downloader.(interface{ CloseIdleConnections() }); ok {
		old.CloseIdleConnections()
	}
	m.downloader = dl
	m.downloaderMu.Unlock()
}

func NewManager(cfg *config.Config) *Manager {
	// Initialize Mongo Clients if configured
	var mongoConfigs []struct{ Name, URI string }
	for _, m := range cfg.Mongo {
		mongoConfigs = append(mongoConfigs, struct{ Name, URI string }{m.Name, m.URI})
	}
	if len(mongoConfigs) > 0 {
		if err := storage.InitMongoClients(mongoConfigs); err != nil {
			slog.Warn("Failed to init mongo clients", logutil.LogKeyError, err)
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
		schedulerSignal: make(chan struct{}, 1),
		activeDownloads: make(map[string]int),
		downloadQueue:   make(chan *downloadRequest, max(globalLimit*8, 64)), // Buffer size
		subscribers:     make(map[<-chan core.Event]chan core.Event),
		urlRegistry:     NewURLStateRegistry(),
		resolveCache:    NewResolveCache(time.Hour, 10000),
		resolveQueue:    make(chan resolveRequest, 128),
		soQueue:         make(chan smallObjectRequest, 128),
		initializedCh:   make(chan struct{}),
	}
	mgr.resolveCtx, mgr.resolveCancel = context.WithCancel(context.Background())
	mgr.soCtx, mgr.soCancel = context.WithCancel(context.Background())
	mgr.cfgVal.Store(cfg)
	tracker := scrape.NewFileTracker(filepath.Join(cfg.Server.WorkDir, "cache", "task"))
	mgr.scrapeDriver = scrape.NewDriver(tracker, scrape.NewDefaultPager())
	if nd, ok := mgr.getDownloader().(core.DomainLimiter); ok {
		nd.ApplyDomainLimits(cfg.Downloader.DomainLimits)
	}
	// Wire up AggregationService with real callbacks
	mgr.aggSvc = NewAggregationService(
		mgr.getAllTasks,
		mgr.searchTaskObjects,
		mgr.countTaskObjects,
		mgr.collectTaskObjects,
	)
	mgr.startedAt = time.Now()
	mgr.maxFailures = 1000
	mgr.failureRecords = make([]FailureRecord, mgr.maxFailures)
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
	if cfg != nil {
		return cfg.FileRoot()
	}
	// Fallback for test / nil config
	wd := config.GetWorkDir()
	return filepath.Join(wd, "downloads")
}

func (m *Manager) currentCfg() *config.Config {
	return m.configSvc.GetConfig()
}

// findTaskConfig looks up the Task config entry by ID.
func (m *Manager) findTaskConfig(id string) *config.Task {
	cfg := m.currentCfg()
	if cfg == nil {
		return nil
	}
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == id {
			return &cfg.Tasks[i]
		}
	}
	return nil
}

// Initialized returns a channel that is closed when Start() completes initial setup.
// Tests should wait on this before interacting with the manager.
func (m *Manager) Initialized() <-chan struct{} {
	return m.initializedCh
}

func cloneStorageQuery(query *core.StorageQuery) *core.StorageQuery {
	if query == nil {
		return &core.StorageQuery{}
	}
	cloned := *query
	cloned.Filter.TaskIDs = append([]string(nil), query.Filter.TaskIDs...)
	cloned.Filter.URLs = append([]string(nil), query.Filter.URLs...)
	cloned.Filter.Statuses = append([]string(nil), query.Filter.Statuses...)
	cloned.Filter.Tags = append([]string(nil), query.Filter.Tags...)
	cloned.Filter.ExcludeIDs = append([]int64(nil), query.Filter.ExcludeIDs...)
	cloned.Filter.IDs = append([]int64(nil), query.Filter.IDs...)
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
	case "random":
		return []core.StorageSort{{Field: "random"}}
	case "tag_match_desc":
		return []core.StorageSort{{Field: "tag_match_desc", Desc: true}, {Field: "date", Desc: true}, {Field: "url"}}
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

func (m *Manager) flushAllStorages() {
	m.tasks.Range(func(key, value any) bool {
		t := value.(core.Task)
		if flusher, ok := t.Storage().(interface{ ForceFlush() error }); ok {
			if err := flusher.ForceFlush(); err != nil {
				slog.Error("Failed to flush storage", logutil.LogKeyTaskID, t.ID(), logutil.LogKeyError, err)
			}
		}
		return true
	})
	slog.Info("All storages flushed")
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
			"title":    obj.GetMetaTitle(),
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
				Statuses: []string{model.StatusCompleted},
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
		return nil, fmt.Errorf("%w", errTaskNotFound)
	}

	result := map[string]any{
		"id":   t.ID(),
		"type": t.Type(),
	}
	if tCfg != nil {
		result["save_dir"] = tCfg.SaveDir
		result["storage"] = tCfg.Storage
		result["extra"] = tCfg.Extra
		if tCfg.ScrapeEnabled != nil {
			result["scrape_enabled"] = *tCfg.ScrapeEnabled
		}
		if tCfg.DownloadEnabled != nil {
			result["download_enabled"] = *tCfg.DownloadEnabled
		}
		if tCfg.SaveSubDir != "" {
			result["save_sub_dir"] = tCfg.SaveSubDir
		}
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
	// Ensure every object has task_type metadata for frontend plugin dispatch
	taskType := t.Type()
	for _, o := range objs {
		o.EnsureTaskType(taskType)
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

// lightCopyObject creates a lightweight copy of a download object, deep-cloning
// Metadata and dropping the large array fields from Extra (files/images/links) so
// payloads stay small. The original object is never mutated.
func lightCopyObject(o *model.DownloadObject, taskType string) *model.DownloadObject {
	o.RLock()
	c := &model.DownloadObject{
		TaskID:   o.TaskID,
		URL:      o.URL,
		ID:       o.ID,
		SavePath: o.SavePath,
		Status:   o.Status,
		Progress: o.Progress,
		Version:  o.Version,
		Metadata: maps.Clone(o.Metadata),
		Extra:    make(map[string]any, len(o.Extra)),
	}
	for k, v := range o.Extra {
		if k == "files" || k == "images" || k == "links" {
			continue
		}
		c.Extra[k] = v
	}
	o.RUnlock()
	c.EnsureTaskType(taskType)
	return c
}

// GetTaskObjectsMeta returns lightweight copies of a task's objects, optionally
// restricted to one content_group, with the large array fields stripped from
// Extra (files/images/links) so the Web UI can build library/navigation views
// without transferring the bulk image file lists. The content_group filter is a
// plain metadata match, served by the {task_id, metadata.content_group} index.
// Originals are never mutated; Metadata is deep-cloned.
func (m *Manager) GetTaskObjectsMeta(id, contentGroup string) ([]*model.DownloadObject, error) {
	t, ok := m.getTask(id)
	if !ok {
		return nil, fmt.Errorf("%w", errTaskNotFound)
	}
	q := &core.StorageQuery{Filter: core.StorageFilter{}, Light: true}
	if contentGroup != "" {
		q.Filter.Metadata = map[string]string{model.MetadataKeyContentGroup: contentGroup}
	}
	objs, err := m.collectTaskObjects(t, q, 200)
	if err != nil {
		return nil, err
	}
	if objs == nil {
		return []*model.DownloadObject{}, nil
	}
	taskType := t.Type()
	out := make([]*model.DownloadObject, 0, len(objs))
	for _, o := range objs {
		out = append(out, lightCopyObject(o, taskType))
	}
	return out, nil
}

// normalizeContentPageLimit 归一化 content 聚合的 page/limit：page 至少为 1；
// limit<=0（"all"）时归位到第 1 页、limit 取 total。快路径与 fallback 共用，保证响应形状一致。
func normalizeContentPageLimit(page, limit, total int64) (int64, int64) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		page = 1
		limit = total
	}
	return page, limit
}

// GetTaskContentGroups returns one representative object per content group for
// a task, grouped by metadata.content_group and sorted by the representative's
// metadata.date descending (date is the generic recency field; e.g. mxs encodes
// the zero-padded chapter id there so each group's newest chapter leads).
// Used by task pages to show a one-card-per-group shelf. Representatives are
// lightweight copies (large array fields stripped) with Extra.group_size holding
// the member count. Response shape matches taskDetails ({objects,total,page,limit}).
func (m *Manager) GetTaskContentGroups(id string, page, limit int64) (map[string]any, error) {
	t, ok := m.getTask(id)
	if !ok {
		return nil, fmt.Errorf("%w", errTaskNotFound)
	}

	// 快路径：存储层原生 content_group 聚合（mongo 聚合管道），一次往返完成分组/排序/分页。
	if gsr, ok := t.Storage().(core.ContentGroupRepresentatives); ok {
		objs, total, err := gsr.ContentGroupRepresentatives(id, "", "", page, limit)
		if err != nil {
			return nil, err
		}
		// 复刻 fallback 的响应归一化，保证响应形状逐字节一致。
		page, limit = normalizeContentPageLimit(page, limit, total)
		for _, o := range objs {
			o.EnsureTaskType(t.Type())
		}
		if objs == nil {
			objs = []*model.DownloadObject{}
		}
		return map[string]any{
			"objects": objs,
			"total":   total,
			"page":    page,
			"limit":   limit,
		}, nil
	}

	objs, err := m.collectTaskObjects(t, &core.StorageQuery{Filter: core.StorageFilter{}, Light: true}, 200)
	if err != nil {
		return nil, err
	}

	// Group objects by content_group.
	type groupAcc struct {
		group   string
		members []*model.DownloadObject
	}
	byGroup := make(map[string]*groupAcc)
	for _, o := range objs {
		g := o.Metadata[model.MetadataKeyContentGroup]
		if g == "" {
			continue
		}
		acc, ok := byGroup[g]
		if !ok {
			acc = &groupAcc{group: g}
			byGroup[g] = acc
		}
		acc.members = append(acc.members, o)
	}

	// Representative = member with the max metadata.date; sort groups by that desc.
	type groupRep struct {
		group string
		rep   *model.DownloadObject
		date  string
		size  int
	}
	reps := make([]groupRep, 0, len(byGroup))
	for _, acc := range byGroup {
		var rep *model.DownloadObject
		var maxDate string
		for _, o := range acc.members {
			d := o.Metadata["date"]
			if rep == nil || d > maxDate {
				rep = o
				maxDate = d
			}
		}
		reps = append(reps, groupRep{group: acc.group, rep: rep, date: maxDate, size: len(acc.members)})
	}
	sort.Slice(reps, func(i, j int) bool {
		if reps[i].date != reps[j].date {
			return reps[i].date > reps[j].date
		}
		return reps[i].group > reps[j].group
	})

	// Paginate.
	total := int64(len(reps))
	page, limit = normalizeContentPageLimit(page, limit, total)
	var offset int64
	if limit > 0 {
		offset = (page - 1) * limit
	}
	start := min(int(offset), len(reps))
	end := min(start+int(limit), len(reps))

	taskType := t.Type()
	out := make([]*model.DownloadObject, 0, end-start)
	for _, r := range reps[start:end] {
		c := lightCopyObject(r.rep, taskType)
		if c.Extra == nil {
			c.Extra = map[string]any{}
		}
		c.Extra["group_size"] = r.size
		out = append(out, c)
	}
	if out == nil {
		out = []*model.DownloadObject{}
	}

	return map[string]any{
		"objects": out,
		"total":   total,
		"page":    page,
		"limit":   limit,
	}, nil
}

// --- Config Management ---

func (m *Manager) UpdateConfig(newCfg *config.Config, audit *AuditInfo) error {
	// Validate before IO — Clone to avoid map race in ValidateAndClamp
	cfgCopy := newCfg.Clone()
	cfgCopy.ValidateAndClamp()
	// Save to file with comment preservation
	if err := m.configSvc.WriteConfigWithComments(newCfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	// Write backup and audit
	m.writeBackupAndAudit(audit)
	// Apply in-memory config
	m.configSvc.StoreConfig(cfgCopy)
	// Reload components
	m.setDownloader(downloader.New(cfgCopy.Downloader))
	// Apply domain limits to new downloader (consistent with NewManager)
	// 先清理旧配置中已移除的域名
	cfg := m.currentCfg()
	if nd, ok := m.getDownloader().(core.DomainLimiter); ok {
		for domain := range cfg.Downloader.DomainLimits {
			if _, exists := cfgCopy.Downloader.DomainLimits[domain]; !exists {
				nd.Remove(domain)
			}
		}
		nd.ApplyDomainLimits(cfgCopy.Downloader.DomainLimits)
	}
	logutil.InitLogger(cfgCopy.Log)
	// Runtime adjustments
	m.adjustGlobalWorkers(newCfg.Downloader.GlobalConcurrent)
	m.applyTaskRuntime(newCfg)

	// Reconcile runtime state from the now-active config
	m.reconcileScheduler(cfgCopy)
	m.reconcileWorkers(cfgCopy)

	// Load missing tasks
	m.loadTasks()
	// Notify
	slog.Info("Configuration updated")
	m.publish(core.Event{Type: core.EventTaskListChange, Payload: nil})
	go m.scan()
	return nil
}

// writeBackupAndAudit writes a config backup and records audit annotations (note, tag).
func (m *Manager) writeBackupAndAudit(audit *AuditInfo) {
	name, err := m.configSvc.writeConfigBackup()
	if err != nil {
		slog.Warn("Failed to write config backup", logutil.LogKeyError, err)
		return
	}
	if audit == nil {
		return
	}
	msg := audit.Message
	if msg == "" {
		msg = "config update"
	}
	if err := m.configSvc.AddConfigNote(name, msg, audit.Author); err != nil {
		slog.Warn("Failed to add config note", logutil.LogKeyError, err, "filename", name, "message", msg)
	}
	if audit.Source != "" {
		if err := m.configSvc.AddConfigTag(name, audit.Source); err != nil {
			slog.Warn("Failed to add config tag", logutil.LogKeyError, err, "filename", name, "tag", audit.Source)
		}
	}
}

// reconcileScheduler starts or stops the scheduler goroutine based on config.
func (m *Manager) reconcileScheduler(cfg *config.Config) {
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
}

// reconcileWorkers starts or stops download workers based on config.
func (m *Manager) reconcileWorkers(cfg *config.Config) {
	workersWanted := cfg.Runtime.Mode != config.RunModeUI && cfg.Runtime.Download.Enabled
	if workersWanted && !m.workersEnabled.Load() {
		m.workersEnabled.Store(true)
		m.adjustGlobalWorkers(cfg.Downloader.GlobalConcurrent)
		slog.Info("Workers enabled via config update")
	} else if !workersWanted && m.workersEnabled.Load() {
		m.workersEnabled.Store(false)
		wc := m.workerCount.Load()
		for range wc {
			m.workerStop <- struct{}{}
		}
		slog.Info("Workers disabled via config update")
	}
}

func (m *Manager) UpdateLogConfig(newLog logutil.LogConfig) error {
	cur := m.GetConfig()
	// Clone to avoid map race in ValidateAndClamp.
	// ValidateAndClamp modifies t.Extra maps in-place, but the original
	// config's task Extra maps may be concurrently read by other goroutines.
	cfgCopy := cur.Clone()
	cfgCopy.Log = newLog
	cfgCopy.ValidateAndClamp()
	if err := config.Save(config.GetConfigFilePath(), cfgCopy); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	logutil.InitLogger(newLog)
	m.configSvc.StoreConfig(cfgCopy)
	return nil
}
