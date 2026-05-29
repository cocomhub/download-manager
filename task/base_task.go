// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"sync/atomic"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/configutil"
	"github.com/cocomhub/download-manager/pkg/dlcore"
	"github.com/cocomhub/download-manager/storage"
)

// HeadersCap is implemented by tasks that support custom download headers.
type HeadersCap interface {
	SetHeaders(map[string]string)
}

// BaseTask provides a shared base for Task implementations, embedding common
// fields (id, saveDir, store, shared registry, mutex) and utility methods
// (UpdateStatus, Close, capability setters) that most tasks need.
//
// Tasks should embed this struct and override Type() and GetDownloadObjects().
type BaseTask struct {
	config.Task
	Options
	logger       *slog.Logger
	dl           core.Downloader
	shared       core.SharedRegistry
	mu           sync.Mutex
	refresherFn  func()
	refresher    *CommonRefresher
	pager        *CommonPager
	pathStrategy core.PathStrategy
	headers      map[string]string

	// Common runtime state
	objects        []*model.DownloadObject
	knownURLs      map[string]bool
	markAsFailed   sync.Map
	concurrency    atomic.Int64
	refreshInt     atomic.Int64
	scrapeMaxPages int // 0 means unlimited
}

func NewBaseTask(cfg *config.Task, opts Options) (*BaseTask, error) {
	if opts.store == nil {
		store, err := storage.NewStorage(cfg.Storage.Type, cfg.Storage.Config)
		if err != nil {
			return nil, fmt.Errorf("task: new storage: %w", err)
		}
		opts.store = store
	}

	t := &BaseTask{
		Task:      *cfg,
		Options:   opts,
		logger:    slog.With("task_id", cfg.ID),
		objects:   make([]*model.DownloadObject, 0),
		knownURLs: map[string]bool{},
	}
	t.concurrency.Store(configutil.GetInt64(cfg.Extra, "max_concurrent", 2))
	t.refreshInt.Store(configutil.GetInt64(cfg.Extra, "refresh_interval", 3600))

	psMode := configutil.GetString(cfg.Extra, "path_strategy", "first_fixed")
	t.pathStrategy = NewPathStrategy(psMode, cfg.SaveDir)
	return t, nil
}

// ID returns the task unique identifier.
func (b *BaseTask) ID() string {
	return b.Task.ID
}

// SaveDir returns the task save directory.
func (b *BaseTask) SaveDir() string {
	return b.Task.SaveDir
}

// GetDownloadHeaders returns the custom HTTP headers for downloads.
func (b *BaseTask) GetDownloadHeaders() map[string]string {
	if b.headers == nil {
		return map[string]string{}
	}
	return b.headers
}

// Type returns the task type. Override this in the embedding task.
func (b *BaseTask) Type() string {
	return "base"
}

// Start 开始任务
func (b *BaseTask) Start() error {
	if b.refresher == nil && b.refresherFn != nil {
		b.refresher = NewCommonRefresher(b.RefreshInterval())
		b.refresher.Start(b.refresherFn)
	}
	return nil
}

// Close flushes the storage (if supported) and stops the refresher (if running).
func (b *BaseTask) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.store != nil {
		if flusher, ok := b.store.(interface{ ForceFlush() error }); ok {
			if err := flusher.ForceFlush(); err != nil {
				b.logger.Error("force flush store failed", "error", err)
				return err
			}
		}
	}
	if b.refresher != nil {
		b.refresher.Stop()
	}
	return nil
}

// UpdateStatus updates the object status, persists to store and shared registry,
// and upserts the object into the runtime list.
func (b *BaseTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj.Status = status

	if err != nil {
		b.logger.Error("Object failed", "url", obj.URL, "error", err)
	} else {
		b.logger.Info("Object status updated", "url", obj.URL, "status", status)
	}

	var storeErr error
	if b.store != nil {
		storeErr = b.store.Update(obj)
		if storeErr != nil {
			b.logger.Error("Failed to update storage", "error", storeErr)
		}
	}
	if b.shared != nil {
		_ = b.shared.Update(obj)
	}
	b.objects = upsertRuntimeObject(b.objects, obj)
	b.knownURLs = rememberRuntimeURLs(b.objects)
	return storeErr
}

// SetSharedRegistry sets the shared registry for cross-task deduplication.
func (b *BaseTask) SetSharedRegistry(reg core.SharedRegistry) {
	b.shared = reg
}

// SetDownloader is a no-op by default. Override in embedding task if needed.
func (b *BaseTask) SetDownloader(dl core.Downloader) {
	b.dl = dl
}

// SetPathStrategy sets the path strategy. Only takes effect if not already set.
func (b *BaseTask) SetPathStrategy(ps core.PathStrategy) {
	if b.pathStrategy == nil && ps != nil {
		b.pathStrategy = ps
	}
}

// SetPager sets the common pager. Only takes effect if not already set.
func (b *BaseTask) SetPager(p *CommonPager) {
	if b.pager == nil && p != nil {
		b.pager = p
	}
}

// SetRefreshFunc sets the refresh func.
func (b *BaseTask) SetRefreshFunc(fn func()) {
	b.refresherFn = fn
}

// SetHeaders sets the custom download headers.
func (b *BaseTask) SetHeaders(h map[string]string) {
	b.headers = h
}

// --- Common runtime object management ---

// GetAllObjects returns a copy of all runtime objects (under lock).
func (b *BaseTask) GetAllObjects(lock bool) []*model.DownloadObject {
	if lock {
		b.mu.Lock()
		defer b.mu.Unlock()
	}
	if b.shared != nil {
		for _, obj := range b.objects {
			b.syncSharedToObjectLocked(obj)
		}
	}
	return b.objects
}

// Storage returns the task's storage backend.
func (b *BaseTask) Storage() core.Storage {
	return b.store
}

// Logger returns the task logger.
func (b *BaseTask) Logger() *slog.Logger {
	return b.logger
}

// Concurrency returns the configured concurrency limit.
func (b *BaseTask) Concurrency() int {
	return int(b.concurrency.Load())
}

// SetConcurrency updates the concurrency limit (0..100).
func (b *BaseTask) SetConcurrency(n int) error {
	if n < 0 || n > 100 {
		return fmt.Errorf("concurrency must be >= 0 and <= 100")
	}
	b.concurrency.Store(int64(n))
	return nil
}

// GetRefreshInterval returns the configured refresh interval in seconds.
func (b *BaseTask) RefreshInterval() int {
	return int(b.refreshInt.Load())
}

// SetRefreshInterval updates the refresh interval (10..86400) and syncs to refresher.
func (b *BaseTask) SetRefreshInterval(sec int) error {
	if sec < 10 || sec > 86400 {
		return fmt.Errorf("refresh interval must be >= 10 and <= 86400")
	}
	b.refreshInt.Store(int64(sec))
	if b.refresher != nil {
		b.refresher.UpdateInterval(int64(sec))
	}
	return nil
}

// MarkAsFailed records an object as permanently failed for this task.
func (b *BaseTask) MarkAsFailed(obj *model.DownloadObject, err error) {
	b.markAsFailed.Store(obj.URL, err)
}

// IsMarkedFailed checks if an object has been marked as failed.
func (b *BaseTask) IsMarkedFailed(url string) bool {
	_, ok := b.markAsFailed.Load(url)
	return ok
}

// CheckAndRestoreStatus tries to restore an object's state from shared registry
// first, then from local storage. This is used when creating objects from items
// to avoid re-downloading already completed items.
func (b *BaseTask) CheckAndRestoreStatus(obj *model.DownloadObject) {
	if b.shared != nil {
		if so, err := b.shared.Get(obj.URL); err == nil && so != nil {
			applySharedState(obj, so)
			return
		}
	}
	if b.store != nil {
		if so, err := b.store.Get(obj.URL); err == nil && so != nil {
			applySharedState(obj, so)
		}
	}
}

// CheckRestoreCompleted is like CheckAndRestoreStatus but only restores if the
// stored object has a completed status. This is used when re-scraping pending/failed
// objects to avoid stale metadata from overwriting fresh content.
func (b *BaseTask) CheckRestoreCompleted(obj *model.DownloadObject) {
	if b.shared != nil {
		if so, err := b.shared.Get(obj.URL); err == nil && so != nil && so.Status == dlcore.StatusCompleted {
			applySharedState(obj, so)
			return
		}
	}
	if b.store != nil {
		if so, err := b.store.Get(obj.URL); err == nil && so != nil && so.Status == dlcore.StatusCompleted {
			applySharedState(obj, so)
		}
	}
}

// GetCachedObject returns a cached object from shared registry or storage, or nil.
func (b *BaseTask) GetCachedObject(url string) *model.DownloadObject {
	if b.shared != nil {
		if so, err := b.shared.Get(url); err == nil && so != nil {
			return so
		}
	}
	if b.store != nil {
		if so, err := b.store.Get(url); err == nil && so != nil {
			return so
		}
	}
	return nil
}

// RememberRuntimeObject upserts an object into the runtime list and updates knownURLs.
func (b *BaseTask) RememberRuntimeObject(obj *model.DownloadObject, lock bool) {
	if obj == nil {
		return
	}
	if lock {
		b.mu.Lock()
		defer b.mu.Unlock()
	}
	b.objects = upsertRuntimeObject(b.objects, obj)
	b.knownURLs = rememberRuntimeURLs(b.objects)
}

// SnapshotRuntimeObjects returns a copy of all runtime objects.
func (b *BaseTask) SnapshotRuntimeObjects(lock bool) []*model.DownloadObject {
	if lock {
		b.mu.Lock()
		defer b.mu.Unlock()
	}
	return append([]*model.DownloadObject(nil), b.objects...)
}

// KnownURLs returns the set of known URLs (under lock).
func (b *BaseTask) KnownURLs() map[string]bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make(map[string]bool, len(b.knownURLs))
	maps.Copy(result, b.knownURLs)
	return result
}

// StorageExistenceMap checks which of the given URLs already exist in storage or runtime.
// Returns a map[url]true for URLs that already exist.
func (b *BaseTask) StorageExistenceMap(urls []string, lock bool) map[string]bool {
	runtimeObjects := b.SnapshotRuntimeObjects(lock)
	return storageExistenceMap(b.store, runtimeObjects, urls)
}

// ProcessNewURLs is the unified dedup entry point for task scrape pipelines.
// Given a list of URLs (typically from one page of a paged scrape), it returns
// the URLs that are NOT yet known (neither in runtime nor in storage), along
// with allKnown — true iff every non-empty URL is already known.
//
// Semantics: allKnown is strictly based on storage/runtime presence; it is
// independent of whether the caller can later successfully construct objects
// for the unknown URLs. Callers that need a circuit breaker against repeated
// downstream failures (e.g. scrapeAndBuild always failing) must add their own
// safeguard on top of this.
func (b *BaseTask) ProcessNewURLs(urls []string) (unknownURLs []string, allKnown bool) {
	existing := b.StorageExistenceMap(urls, true)
	allKnown = true
	for _, u := range urls {
		if u == "" {
			continue
		}
		if existing[u] {
			continue
		}
		allKnown = false
		unknownURLs = append(unknownURLs, u)
	}
	return unknownURLs, allKnown
}

// PersistTaskObject saves an object to both store and shared registry.
func (b *BaseTask) PersistTaskObject(obj *model.DownloadObject) {
	persistTaskObject(b.store, b.shared, obj)
}

// ResetZombieState checks for zombie downloading states in the given object
// and resets them to pending if found.
func (b *BaseTask) ResetZombieState(obj *model.DownloadObject) {
	if obj.Status == dlcore.StatusDownloading {
		b.logger.Warn("Found zombie downloading state, resetting to pending", "url", obj.URL)
		obj.Status = dlcore.StatusPending
		if b.store != nil {
			if err := b.store.Update(obj); err != nil {
				b.logger.Error("Failed to reset zombie state", "error", err)
			}
		}
	}
}

// Scrape implements core.ScrapeCap by delegating to CommonPager.ScrapeAllPages.
// If maxPages > 0, stops after that many pages; otherwise unlimited.
// Honors ctx for cancellation.
func (b *BaseTask) Scrape(ctx context.Context) error {
	if b.pager == nil {
		return nil
	}
	b.pager.ScrapeAllPages(ctx, b.scrapeMaxPages)
	return nil
}

// SetScrapeMaxPages sets a page limit for Scrape(). 0 means unlimited.
func (b *BaseTask) SetScrapeMaxPages(n int) {
	b.scrapeMaxPages = n
}

// DefaultRefreshLatest is the default refresh function for tasks with a pager.
// It calls RefreshPager and remembers new objects.
func (b *BaseTask) DefaultRefreshLatest() {
	if !b.HasPager() {
		return
	}
	newAny, err := b.RefreshPager()
	if err != nil {
		b.logger.Error("Refresh failed", "error", err)
		return
	}
	if len(newAny) == 0 {
		return
	}
	for i := range newAny {
		b.RememberRuntimeObject(newAny[i].(*model.DownloadObject), true)
	}
	b.logger.Info("Refresh finished", "new_items", len(newAny))
}

// HasPager returns whether a CommonPager is configured.
func (b *BaseTask) HasPager() bool {
	return b.pager != nil
}

// LoadPendingFromStorage queries storage for pending and failed objects,
// merges them into the runtime list, and returns the list.
func (b *BaseTask) LoadPendingFromStorage(limit int64) []*model.DownloadObject {
	if b.store == nil {
		return nil
	}
	stored, err := b.store.Search(&core.StorageQuery{
		Filter: core.StorageFilter{
			TaskIDs:  []string{b.ID()},
			Statuses: []string{dlcore.StatusPending, dlcore.StatusFailed},
		},
		Sort:  []core.StorageSort{{Field: "date", Desc: true}, {Field: "url"}},
		Limit: limit,
	})
	if err != nil || len(stored) == 0 {
		return nil
	}
	for _, obj := range stored {
		b.RememberRuntimeObject(obj, true)
	}
	return stored
}

// --- Path resolution ---

// ResolvePath resolves save paths using the configured path strategy.
// Encapsulates pathStrategy.Resolve(b.SaveDir(), b.ID(), title, fileType).
func (b *BaseTask) ResolvePath(title, fileType string) (videoPath, imagePath string) {
	return b.pathStrategy.Resolve(b.SaveDir(), b.ID(), title, fileType)
}

// --- Object persistence update ---

// FlushObject persists an updated object to both store and shared registry.
// Use this for updating existing objects (e.g. after resolve), as opposed to
// PersistTaskObject which is for newly created objects.
func (b *BaseTask) FlushObject(obj *model.DownloadObject) {
	if b.store != nil {
		_ = b.store.Update(obj)
	}
	if b.shared != nil {
		_ = b.shared.Update(obj)
	}
}

// --- Pager refresh ---

// RefreshPager delegates to the pager's RefreshLatest method.
// Returns new items discovered during the refresh.
// If no pager is configured, returns nil, nil.
func (b *BaseTask) RefreshPager() ([]any, error) {
	if b.pager == nil {
		return nil, nil
	}
	return b.pager.RefreshLatest()
}

// --- Lock delegation ---

// WithLock executes fn while holding the mutex.
// Provides safe lock proxy for sub-packages that cannot access mu directly.
func (b *BaseTask) WithLock(fn func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	fn()
}

// WithObjectsLock executes fn while holding the mutex, passing the runtime objects list.
// Replaces the pattern of manually locking mu and iterating t.objects.
func (b *BaseTask) WithObjectsLock(fn func([]*model.DownloadObject)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	fn(b.objects)
}

// RuntimeObjectCount returns the number of runtime objects.
// Not thread-safe; intended for logging and diagnostics.
func (b *BaseTask) RuntimeObjectCount() int {
	return len(b.objects)
}

// --- Convenience methods for sub-package migration ---

// --- Convenience methods for sub-package migration ---
// Used by tasks that need direct download access (e.g. tktube prefetch).
func (b *BaseTask) Downloader() core.Downloader {
	return b.dl
}

// GetSharedObject retrieves an object from the shared registry by URL.
// Returns nil if the object is not found or the registry is not set.
func (b *BaseTask) GetSharedObject(url string) *model.DownloadObject {
	if b.shared == nil {
		return nil
	}
	if so, err := b.shared.Get(url); err == nil && so != nil {
		return so
	}
	return nil
}

// SyncSharedToObject copies the shared registry state into obj.
// Used by tasks that need to refresh an object from the shared registry.
func (b *BaseTask) SyncSharedToObject(obj *model.DownloadObject) {
	if b.shared == nil || obj == nil {
		return
	}
	if so, err := b.shared.Get(obj.URL); err == nil && so != nil {
		applySharedState(obj, so)
	}
}

func (b *BaseTask) syncSharedToObjectLocked(obj *model.DownloadObject) {
	if b.shared == nil || obj == nil {
		return
	}
	if so, err := b.shared.Get(obj.URL); err == nil && so != nil {
		applySharedState(obj, so)
	}
}

func applySharedState(dst, src *model.DownloadObject) {
	if dst == nil || src == nil {
		return
	}
	dst.Status = src.Status
	dst.Progress = src.Progress
	if src.Metadata != nil {
		if dst.Metadata == nil {
			dst.Metadata = make(map[string]string, len(src.Metadata))
		}
		maps.Copy(dst.Metadata, src.Metadata)
	}
	if src.Extra != nil {
		if dst.Extra == nil {
			dst.Extra = make(map[string]any, len(src.Extra))
		}
		maps.Copy(dst.Extra, src.Extra)
	}
}

// CountObjects returns the count of objects matching the given statuses from storage.
func (b *BaseTask) CountObjects(statuses []string) (int64, error) {
	if b.store == nil {
		return 0, nil
	}
	return b.store.Count(&core.StorageQuery{
		Filter: core.StorageFilter{
			TaskIDs:  []string{b.ID()},
			Statuses: statuses,
		},
	})
}

// MoveObject moves an object identified by url to a new index in the runtime list.
func (b *BaseTask) MoveObject(url string, newIndex int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	currentIndex := -1
	for i, obj := range b.objects {
		if obj.URL == url {
			currentIndex = i
			break
		}
	}
	if currentIndex == -1 {
		return fmt.Errorf("object not found")
	}
	if newIndex < 0 {
		newIndex = 0
	}
	if newIndex >= len(b.objects) {
		newIndex = len(b.objects) - 1
	}
	if currentIndex == newIndex {
		return nil
	}

	obj := b.objects[currentIndex]
	b.objects = append(b.objects[:currentIndex], b.objects[currentIndex+1:]...)
	b.objects = append(b.objects[:newIndex], append([]*model.DownloadObject{obj}, b.objects[newIndex:]...)...)
	return nil
}
