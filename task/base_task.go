// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"sync/atomic"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"
)

// PagingCap is implemented by tasks that support CommonPager.
type PagingCap interface {
	SetPager(*CommonPager)
}

// RefreshingCap is implemented by tasks that support CommonRefresher.
type RefreshingCap interface {
	SetRefresher(*CommonRefresher)
}

// PathStrategyCap is implemented by tasks that use PathStrategy.
type PathStrategyCap interface {
	SetPathStrategy(core.PathStrategy)
}

// HeadersCap is implemented by tasks that support custom download headers.
type HeadersCap interface {
	SetHeaders(map[string]string)
}

// ConcurrencyCap is implemented by tasks that have configurable concurrency.
type ConcurrencyCap interface {
	GetConcurrency() int
	SetConcurrency(n int) error
}

// RefreshIntervalCap is implemented by tasks that have configurable refresh interval.
type RefreshIntervalCap interface {
	GetRefreshInterval() int
	SetRefreshInterval(sec int) error
}

// AllObjectsCap is implemented by tasks that can list all their runtime objects.
type AllObjectsCap interface {
	GetAllObjects() []*model.DownloadObject
}

// BaseTask provides a shared base for Task implementations, embedding common
// fields (id, saveDir, store, shared registry, mutex) and utility methods
// (UpdateStatus, Close, capability setters) that most tasks need.
//
// Tasks should embed this struct and override Type() and GetDownloadObjects().
type BaseTask struct {
	id           string
	saveDir      string
	dl           core.Downloader
	store        core.Storage
	shared       core.SharedRegistry
	mu           sync.Mutex
	refresher    *CommonRefresher
	pager        *CommonPager
	pathStrategy core.PathStrategy
	headers      map[string]string

	// Common runtime state
	objects      []*model.DownloadObject
	knownURLs    map[string]bool
	initialized  atomic.Int32
	markAsFailed sync.Map
	concurrency  int
	refreshInt   int
}

func NewBaseTask(id, saveDir string, store core.Storage) BaseTask {
	return BaseTask{
		id:        id,
		saveDir:   saveDir,
		store:     store,
		objects:   make([]*model.DownloadObject, 0),
		knownURLs: map[string]bool{},
	}
}

// ID returns the task unique identifier.
func (b *BaseTask) ID() string {
	return b.id
}

// SaveDir returns the task save directory.
func (b *BaseTask) SaveDir() string {
	return b.saveDir
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

// Close flushes the storage (if supported) and stops the refresher (if running).
func (b *BaseTask) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.store != nil {
		if flusher, ok := b.store.(interface{ ForceFlush() error }); ok {
			if err := flusher.ForceFlush(); err != nil {
				slog.Error("force flush store failed", "task_id", b.id, "error", err)
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
		slog.Error("Object failed", "task_id", b.id, "url", obj.URL, "error", err)
	} else {
		slog.Info("Object status updated", "task_id", b.id, "url", obj.URL, "status", status)
	}

	var storeErr error
	if b.store != nil {
		storeErr = b.store.Update(obj)
		if storeErr != nil {
			slog.Error("Failed to update storage", "task_id", b.id, "error", storeErr)
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

// SetRefresher sets the common refresher. Only takes effect if not already set.
func (b *BaseTask) SetRefresher(r *CommonRefresher) {
	if b.refresher == nil && r != nil {
		b.refresher = r
	}
}

// SetHeaders sets the custom download headers.
func (b *BaseTask) SetHeaders(h map[string]string) {
	b.headers = h
}

// --- Common runtime object management ---

// GetAllObjects returns a copy of all runtime objects (under lock).
func (b *BaseTask) GetAllObjects() []*model.DownloadObject {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.objects
}

// GetStorage returns the task's storage backend.
func (b *BaseTask) GetStorage() core.Storage {
	return b.store
}

// GetConcurrency returns the configured concurrency limit.
func (b *BaseTask) GetConcurrency() int {
	return b.concurrency
}

// SetConcurrency updates the concurrency limit (0..100).
func (b *BaseTask) SetConcurrency(n int) error {
	if n < 0 || n > 100 {
		return fmt.Errorf("concurrency must be >= 0 and <= 100")
	}
	b.mu.Lock()
	b.concurrency = n
	b.mu.Unlock()
	return nil
}

// GetRefreshInterval returns the configured refresh interval in seconds.
func (b *BaseTask) GetRefreshInterval() int {
	return b.refreshInt
}

// SetRefreshInterval updates the refresh interval (10..86400) and syncs to refresher.
func (b *BaseTask) SetRefreshInterval(sec int) error {
	if sec < 10 || sec > 86400 {
		return fmt.Errorf("refresh interval must be >= 10 and <= 86400")
	}
	b.mu.Lock()
	b.refreshInt = sec
	b.mu.Unlock()
	if b.refresher != nil {
		b.refresher.UpdateInterval(sec)
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
			*obj = *so
			return
		}
	}
	if b.store != nil {
		if so, err := b.store.Get(obj.URL); err == nil && so != nil {
			*obj = *so
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
// Caller must NOT hold b.mu; this method acquires the lock internally.
func (b *BaseTask) RememberRuntimeObject(obj *model.DownloadObject) {
	if obj == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.objects = upsertRuntimeObject(b.objects, obj)
	b.knownURLs = rememberRuntimeURLs(b.objects)
}

// SnapshotRuntimeObjects returns a copy of all runtime objects.
// Caller must NOT hold b.mu; this method acquires the lock internally.
func (b *BaseTask) SnapshotRuntimeObjects() []*model.DownloadObject {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*model.DownloadObject(nil), b.objects...)
}

// SnapshotRuntimeObjectsLocked returns a copy of all runtime objects.
// Caller MUST hold b.mu; this method does NOT acquire the lock.
func (b *BaseTask) SnapshotRuntimeObjectsLocked() []*model.DownloadObject {
	return append([]*model.DownloadObject(nil), b.objects...)
}

// UpsertRuntimeObjectLocked upserts an object into the runtime list and updates knownURLs.
// Caller MUST hold b.mu.
func (b *BaseTask) UpsertRuntimeObjectLocked(obj *model.DownloadObject) {
	b.objects = upsertRuntimeObject(b.objects, obj)
	b.knownURLs = rememberRuntimeURLs(b.objects)
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
func (b *BaseTask) StorageExistenceMap(urls []string) map[string]bool {
	runtimeObjects := b.SnapshotRuntimeObjects()
	return storageExistenceMap(b.store, runtimeObjects, urls)
}

// StorageExistenceMapLocked checks which of the given URLs already exist in storage or runtime.
// Caller MUST hold b.mu; this method does NOT acquire the lock.
func (b *BaseTask) StorageExistenceMapLocked(urls []string) map[string]bool {
	runtimeObjects := b.SnapshotRuntimeObjectsLocked()
	return storageExistenceMap(b.store, runtimeObjects, urls)
}

// PersistTaskObject saves an object to both store and shared registry.
func (b *BaseTask) PersistTaskObject(obj *model.DownloadObject) {
	persistTaskObject(b.store, b.shared, obj)
}

// ResetZombieState checks for zombie downloading states in the given object
// and resets them to pending if found.
func (b *BaseTask) ResetZombieState(obj *model.DownloadObject) {
	if obj.Status == dlcore.StatusDownloading {
		slog.Warn("Found zombie downloading state, resetting to pending", "task_id", b.id, "url", obj.URL)
		obj.Status = dlcore.StatusPending
		if b.store != nil {
			if err := b.store.Update(obj); err != nil {
				slog.Error("Failed to reset zombie state", "task_id", b.id, "error", err)
			}
		}
	}
}
