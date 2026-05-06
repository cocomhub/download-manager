// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/downloader"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"
	"github.com/cocomhub/download-manager/pkg/titlegroup"

	"github.com/PuerkitoBio/goquery"
	"github.com/dop251/goja"
)

// TktubeTask implements core.Task for Tktube
type TktubeTask struct {
	id          string
	taskType    string // "tag", "model", "search"
	keyword     string
	pageStart   int // Configured start page (usually 1)
	pageEnd     int // Configured end page (can be overridden by auto-detection)
	saveDir     string
	concurrency int
	refreshInt  int // Refresh interval in seconds

	objects       []*model.DownloadObject
	store         core.Storage
	shared        core.SharedRegistry
	mu            sync.Mutex
	initialized   atomic.Int32
	prefetchQueue chan *model.DownloadObject
	prefetchRate  int
	pathStrategy  core.PathStrategy
	refresher     *CommonRefresher
	pager         *CommonPager

	// Set to track existing URLs for quick lookup
	knownURLs map[string]bool

	markAsFailed sync.Map
	dl           core.Downloader
}

// Ensure TktubeTask implements core.Task
var _ core.Task = &TktubeTask{}

// SetPathStrategy allows factory to inject a default path strategy when not set
func (t *TktubeTask) SetPathStrategy(ps core.PathStrategy) {
	if t.pathStrategy == nil && ps != nil {
		t.pathStrategy = ps
	}
}

// SetRefresher allows factory to inject a default refresher when not set
func (t *TktubeTask) SetRefresher(r *CommonRefresher) {
	if t.refresher == nil && r != nil {
		t.refresher = r
	}
}

func NewTktubeTask(cfg config.Task, store core.Storage) (*TktubeTask, error) {
	extra := cfg.Extra
	if extra == nil {
		extra = make(map[string]any)
	}

	getString := func(key, def string) string {
		if v, ok := extra[key].(string); ok {
			return v
		}
		return def
	}
	getInt := func(key string, def int) int {
		if v, ok := extra[key].(int); ok {
			return v
		}
		if v, ok := extra[key].(float64); ok {
			return int(v)
		}
		return def
	}
	getBool := func(key string, def bool) bool {
		if v, ok := extra[key].(bool); ok {
			return v
		}
		return def
	}

	subtype := getString("subtype", "tag")
	keyword := getString("keyword", "")
	concurrency := getInt("max_concurrent", 2)
	refreshInt := getInt("refresh_interval", 3600) // Default 1 hour
	saveDir := cfg.SaveDir
	if getBool("save_dir_add_keyword", false) {
		saveDir = filepath.Join(cfg.SaveDir, keyword)
	}
	psMode := getString("path_strategy", "first_fixed")

	t := &TktubeTask{
		id:            cfg.ID,
		taskType:      subtype,
		keyword:       keyword,
		pageStart:     1,
		pageEnd:       1,
		saveDir:       saveDir,
		concurrency:   concurrency,
		refreshInt:    refreshInt,
		objects:       make([]*model.DownloadObject, 0),
		store:         store,
		prefetchQueue: make(chan *model.DownloadObject, 100), // Buffer
		prefetchRate:  getInt("prefetch_rate", 10),
		knownURLs:     make(map[string]bool),
	}
	t.pathStrategy = NewPathStrategy(psMode, saveDir)
	// Common pager setup
	t.pager = NewCommonPager(PageFuncs{
		BuildPageURL:    t.buildPageURL,
		RunScraper:      t.runScraper,
		ParseHomePage:   func(html string) (any, error) { return t.parseHomePage(html) },
		ParseTotalPages: t.parseTotalPages,
		ProcessItems: func(items any) ([]any, bool) {
			vs, _ := items.([]videoItem)
			t.mu.Lock()
			runtimeObjects := append([]*model.DownloadObject(nil), t.objects...)
			t.mu.Unlock()
			existing := storageExistenceMap(t.store, runtimeObjects, videoItemURLs(vs))
			var pageNew []*model.DownloadObject
			allKnown := true
			for _, v := range vs {
				if existing[v.href] {
					continue
				}
				allKnown = false
				obj := t.createObjectFromVideoItem(v)
				persistTaskObject(t.store, t.shared, obj)
				t.rememberRuntimeObject(obj)
				pageNew = append(pageNew, obj)
				t.queuePrefetch(obj)
			}
			out := make([]any, len(pageNew))
			for i := range pageNew {
				out[i] = pageNew[i]
			}
			return out, allKnown
		},
	})

	// Start prefetch workers
	go t.startPrefetchWorkers(3) // 3 parallel prefetchers
	// Start periodic refresher
	t.refresher = NewCommonRefresher(refreshInt)
	t.refresher.Start(t.refreshLatest)

	return t, nil
}

func (t *TktubeTask) SetDownloader(d core.Downloader) {
	t.dl = d
}

func (t *TktubeTask) SetSharedRegistry(reg core.SharedRegistry) {
	t.shared = reg
}

func (t *TktubeTask) GetStorage() core.Storage {
	return t.store
}

func (t *TktubeTask) ID() string {
	return t.id
}

func (t *TktubeTask) Type() string {
	return TypeTktube
}

func (t *TktubeTask) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Flush storage if supported
	if t.store != nil {
		if flusher, ok := t.store.(interface{ ForceFlush() error }); ok {
			if err := flusher.ForceFlush(); err != nil {
				slog.Error("Failed to flush storage", "task_id", t.id, "error", err)
			}
		}
	}
	if t.refresher != nil {
		t.refresher.Stop()
	}
	return nil
}

func (t *TktubeTask) GetDownloadHeaders() map[string]string {
	return map[string]string{}
}

func (t *TktubeTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	// 1. Initialize (Scrape all pages) if not done
	if t.initialized.CompareAndSwap(0, -1) {
		// Start initialization in background to avoid blocking
		go t.scrapeAllPages()
		// Return empty list while initializing
		return []*model.DownloadObject{}, nil
	}

	if t.initialized.Load() != 1 {
		return []*model.DownloadObject{}, nil
	}

	t.mu.Lock()
	t.queuePendingPrefetches()
	runtimeObjects := append([]*model.DownloadObject(nil), t.objects...)
	t.mu.Unlock()

	// 2. Return pending objects that are ready for download
	activeCount := 0
	if t.store != nil {
		count, err := t.store.Count(&core.StorageQuery{
			Filter: core.StorageFilter{
				TaskIDs:  []string{t.id},
				Statuses: []string{dlcore.StatusDownloading},
			},
		})
		if err == nil {
			activeCount = count
		}
	}
	if activeCount == 0 {
		for _, obj := range runtimeObjects {
			if obj.Status == dlcore.StatusDownloading {
				activeCount++
			}
		}
	}
	// if activeCount != 0 {
	// 	slog.Debug("Active count", "task_id", t.id, "count", activeCount)
	// }

	candidates := make([]*model.DownloadObject, 0)
	toResolve := make([]*model.DownloadObject, 0)

	queryLimit := max(t.concurrency*3+8, 16)
	objects := runtimeObjects
	if t.store != nil {
		if stored, err := t.store.Search(&core.StorageQuery{
			Filter: core.StorageFilter{
				TaskIDs:  []string{t.id},
				Statuses: []string{dlcore.StatusPending, dlcore.StatusFailed},
			},
			Sort:  []core.StorageSort{{Field: "date", Desc: true}, {Field: "url"}},
			Limit: queryLimit,
		}); err == nil {
			objects = stored
			for _, obj := range stored {
				t.rememberRuntimeObject(obj)
			}
		}
	}

	// Collect candidates
	for _, obj := range objects {
		if obj.Status != dlcore.StatusCompleted && obj.Status != dlcore.StatusCancelled {
			// Check if failed
			if _, ok := t.markAsFailed.Load(obj.URL); ok {
				continue
			}
			// We look ahead a bit more to ensure we have enough resolved objects
			if len(candidates)+len(toResolve)+activeCount < t.concurrency*2+2 {
				// Check if resolved
				if _, hasFiles := obj.Extra["files"]; hasFiles {
					candidates = append(candidates, obj)
				} else {
					toResolve = append(toResolve, obj)
				}
			} else {
				// Stop if we have enough candidates
				break
			}
		}
	}

	// Resolve in parallel
	if len(toResolve) > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex // To protect append to candidates

		// Limit resolution concurrency to avoid flooding
		sem := make(chan struct{}, 5)

		for _, obj := range toResolve {
			wg.Add(1)
			go func(o *model.DownloadObject) {
				defer wg.Done()
				sem <- struct{}{}        // Acquire
				defer func() { <-sem }() // Release

				if err := t.resolveVideoDetails(o); err != nil {
					slog.Error("Failed to resolve video details", "task_id", t.id, "url", o.URL, "error", err)
					t.UpdateStatus(o, dlcore.StatusFailed, err)
				} else {
					mu.Lock()
					candidates = append(candidates, o)
					mu.Unlock()
				}
			}(obj)
		}
		wg.Wait()
	}

	return candidates, nil
}

func (t *TktubeTask) MarkAsFailed(obj *model.DownloadObject, err error) {
	t.markAsFailed.Store(obj.URL, err)
}

// ResolveObject explicitly resolves an object (exposed for Manager)
func (t *TktubeTask) ResolveObject(obj *model.DownloadObject) error {
	// Check shared state for resolved files first
	if t.shared != nil {
		if so, err := t.shared.Get(obj.URL); err == nil && so != nil {
			if files, ok := so.Extra["files"]; ok {
				t.mu.Lock()
				obj.Extra["files"] = files
				t.mu.Unlock()
				return nil
			}
		}
	}
	// Check if already resolved
	if _, hasFiles := obj.Extra["files"]; hasFiles {
		return nil
	}
	return t.resolveVideoDetails(obj)
}

// --- Scraping Logic ---

func (t *TktubeTask) scrapeAllPages() {
	defer t.initialized.Store(1)

	t.mu.Lock()
	defer t.mu.Unlock()

	// First scrape page 1 to get total pages and initial items
	page1URL := t.buildPageURL(t.pageStart)
	slog.Info("Scraping first page to detect total pages", "task_id", t.id, "url", page1URL)

	html, err := t.runScraper(page1URL)
	if err != nil {
		slog.Error("Failed to scrape first page", "error", err)
		return
	}

	// Parse total pages
	totalPages := t.parseTotalPages(html)
	if totalPages > t.pageEnd {
		slog.Info("Detected more pages", "task_id", t.id, "old_end", t.pageEnd, "new_end", totalPages)
		t.pageEnd = totalPages
	}

	// Parse items from page 1
	items1, err := t.parseHomePage(html)
	if err == nil {
		t.addVideoItems(items1)
	} else {
		slog.Error("Failed to parse first page", "task_id", t.id, "error", err)
		return
	}

	// Scrape remaining pages
	// We iterate from start+1 to end.
	// Note: If we want "newest first" and pages are 1..N (newest on 1),
	// we have page 1.
	// If we want to scrape EVERYTHING, we just loop.
	for i := t.pageStart + 1; i <= t.pageEnd; i++ {
		url := t.buildPageURL(i)
		slog.Info("Scraping All pages", "task_id", t.id, "page", i, "url", url)

		html, err := t.runScraper(url)
		if err != nil {
			slog.Error("Failed to scrape page", "task_id", t.id, "page", i, "error", err)
			continue
		}

		items, err := t.parseHomePage(html)
		if err != nil {
			slog.Error("Failed to parse page", "task_id", t.id, "page", i, "error", err)
			continue
		}

		t.addVideoItems(items)
	}

	slog.Info("Initialization done", "task_id", t.id, "total_objects", len(t.objects))
}

func (t *TktubeTask) refreshLatest() {
	slog.Info("Refreshing task", "task_id", t.id, "keyword", t.keyword)
	newAny, err := t.pager.RefreshLatest()
	if err != nil {
		slog.Error("Refresh failed", "task_id", t.id, "error", err)
		return
	}
	if len(newAny) > 0 {
		for i := range newAny {
			t.rememberRuntimeObject(newAny[i].(*model.DownloadObject))
		}

		slog.Info("Refresh finished", "task_id", t.id, "new_items", len(newAny))
	} else {
		slog.Info("No new items found", "task_id", t.id)
	}
}

func (t *TktubeTask) addVideoItems(items []videoItem) {
	existing := storageExistenceMap(t.store, t.snapshotRuntimeObjects(), videoItemURLs(items))
	for _, v := range items {
		if existing[v.href] {
			continue
		}

		obj := t.createObjectFromVideoItem(v)
		persistTaskObject(t.store, t.shared, obj)
		t.rememberRuntimeObject(obj)
		t.queuePrefetch(obj)
	}
}

func (t *TktubeTask) parseTotalPages(html string) int {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return 0
	}

	// Find .pagination .last
	// <li class="last"><a ... data-parameters="...;from:24">最後</a></li>
	// Extract 'from:24'

	var lastPage int
	doc.Find(".pagination .last a").Each(func(i int, s *goquery.Selection) {
		params, exists := s.Attr("data-parameters")
		if exists {
			// "sort_by:post_date;from:24"
			parts := strings.SplitSeq(params, ";")
			for p := range parts {
				if after, ok := strings.CutPrefix(p, "from:"); ok {
					valStr := after
					if val, err := strconv.Atoi(valStr); err == nil {
						lastPage = val
					}
				}
			}
		}
	})

	// Fallback: check .page links if .last not found (e.g. few pages)
	if lastPage == 0 {
		doc.Find(".pagination .page a").Each(func(i int, s *goquery.Selection) {
			params, exists := s.Attr("data-parameters")
			if exists {
				parts := strings.SplitSeq(params, ";")
				for p := range parts {
					if after, ok := strings.CutPrefix(p, "from:"); ok {
						valStr := after
						if val, err := strconv.Atoi(valStr); err == nil {
							if val > lastPage {
								lastPage = val
							}
						}
					}
				}
			}
		})
	}

	return lastPage
}

func (t *TktubeTask) queuePendingPrefetches() {
	for _, obj := range t.objects {
		if obj.Status == dlcore.StatusPending {
			// Check if prefetch needed
			_, hasLocalPreview := obj.Extra["local_preview"]
			if !hasLocalPreview {
				t.queuePrefetch(obj)
			}
		}
	}
}

func (t *TktubeTask) queuePrefetch(obj *model.DownloadObject) {
	select {
	case t.prefetchQueue <- obj:
	default:
		// Queue full, ignore
	}
}

// --- Prefetch Logic ---

func (t *TktubeTask) startPrefetchWorkers(count int) {
	for i := range count {
		go func(workerID int) {
			for obj := range t.prefetchQueue {
				if t.prefetchRate > 0 {
					time.Sleep(time.Duration(1000/t.prefetchRate) * time.Millisecond)
				}
				t.prefetchAssets(obj)
			}
		}(i)
	}
}

func (t *TktubeTask) prefetchAssets(obj *model.DownloadObject) {
	// Don't prefetch if already completed or downloading main
	if obj.Status == dlcore.StatusCompleted || obj.Status == dlcore.StatusDownloading || obj.Status == dlcore.StatusCancelled {
		return
	}

	// Check if already prefetched (double check)
	t.mu.Lock()
	_, hasPreview := obj.Extra["local_preview"]
	_, hasCover := obj.Extra["local_cover"]
	t.mu.Unlock()

	if hasPreview && hasCover {
		return
	}

	// 1. Preview Video
	previewURL, _ := obj.Extra["preview_url"].(string)
	if previewURL != "" && !hasPreview {
		baseName := strings.ReplaceAll(obj.Metadata["title"], "/", "_")
		path := filepath.Join(t.saveDir, baseName+"_preview.mp4")

		if err := t.downloadFile(previewURL, path); err == nil {
			t.mu.Lock()
			obj.Extra["local_preview"] = path
			if t.store != nil {
				t.store.Update(obj)
			}
			t.mu.Unlock()
			slog.Debug("Prefetched preview", "task_id", t.id, "title", obj.Metadata["title"])
		} else {
			slog.Warn("Failed to prefetch preview, retrying later", "task_id", t.id, "url", previewURL, "error", err)
			// Re-queue for retry?
			// Simple backoff or re-add to queue
			// To avoid infinite loop on bad URL, maybe check retry count?
			// For now, just re-queue with non-blocking
			go func() {
				time.Sleep(10 * time.Second)
				t.queuePrefetch(obj)
			}()
			return // Don't try cover if preview failed? Or try cover anyway? Try cover.
		}
	}

	// 2. Cover Image
	thumbURL, _ := obj.Extra["thumb_url"].(string)
	if thumbURL != "" && !hasCover {
		baseName := strings.ReplaceAll(obj.Metadata["title"], "/", "_")
		path := filepath.Join(t.saveDir, baseName+"_thumb.jpg")

		if err := t.downloadFile(thumbURL, path); err == nil {
			t.mu.Lock()
			obj.Extra["local_cover"] = path
			if t.store != nil {
				t.store.Update(obj)
			}
			t.mu.Unlock()
			slog.Debug("Prefetched cover", "task_id", t.id, "title", obj.Metadata["title"])
		} else {
			go func() {
				time.Sleep(10 * time.Second)
				t.queuePrefetch(obj)
			}()
		}
	}
}

func (t *TktubeTask) downloadFile(url, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	if t.dl == nil {
		return t.simpleDownload(url, path)
	}

	obj := &model.DownloadObject{
		TaskID:   t.id,
		URL:      url,
		SavePath: path,
		Metadata: map[string]string{"type": "image"},
		Extra:    map[string]any{},
		Status:   dlcore.StatusPending,
	}
	return t.dl.Download(obj, t.GetDownloadHeaders())
}

func (t *TktubeTask) simpleDownload(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	// Write to temp first
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		f.Close()
		return err
	}

	f.Write(buf.Bytes())
	f.Close()

	modTimeStr := resp.Header.Get("Last-Modified")
	if modTimeStr != "" {
		modTime, err := time.Parse(time.RFC1123, modTimeStr)
		if err == nil {
			os.Chtimes(tmp, modTime, modTime)
		}
	}

	return os.Rename(tmp, path)
}

func (t *TktubeTask) createObjectFromVideoItem(v videoItem) *model.DownloadObject {
	// Basic object with metadata from list page
	videoPath, _ := t.pathStrategy.Resolve(t.saveDir, t.id, v.title, "video")
	group := titlegroup.TKTContentGroupKey(v.title, v.href)

	obj := &model.DownloadObject{
		TaskID:   t.id,
		URL:      v.href, // Page URL as ID
		SavePath: videoPath,
		Metadata: map[string]string{
			"title":         v.title,
			"type":          "composite",
			"task_type":     t.Type(),
			"duration":      v.duration,
			"date":          v.date,
			"content_group": group,
		},
		Extra: map[string]any{
			"preview_url": v.previewURL,
			"thumb_url":   v.thumbURL,
		},
		Status: dlcore.StatusPending,
	}

	// Deduplication / Restore Status from DB
	t.checkAndRestoreStatus(obj)

	return obj
}

func (t *TktubeTask) resolveVideoDetails(obj *model.DownloadObject) error {
	slog.Info("Resolving video details", "task_id", t.id, "title", obj.Metadata["title"])
	videoInfo, err := t.parseVideoPage(obj.URL)
	if err != nil {
		if err == ErrNoFlashvars {
			t.MarkAsFailed(obj, err)
			return err
		}
		return err
	}

	videoPath, imagePath := t.pathStrategy.Resolve(t.saveDir, t.id, videoInfo.title, "video")

	// Main Video Download
	// We also include the High Res Image here
	files := []map[string]string{
		{
			"url":  videoInfo.imageURL,
			"path": imagePath,
			"type": "image",
		},
		{
			"url":  videoInfo.videoURL,
			"path": videoPath,
			"type": "video",
		},
	}

	t.mu.Lock()
	obj.Extra["tags"] = videoInfo.tags
	if _, ok := obj.Extra["files"]; !ok {
		obj.Extra["files"] = files
	}
	t.mu.Unlock()

	// Update storage
	if t.store != nil {
		t.store.Update(obj)
	}
	// Update shared registry
	if t.shared != nil {
		t.shared.Update(obj)
	}

	return nil
}

func (t *TktubeTask) checkAndRestoreStatus(obj *model.DownloadObject) {
	// Prefer shared registry
	if t.shared != nil {
		if storedObj, err := t.shared.Get(obj.URL); err == nil && storedObj != nil {
			*obj = *storedObj
			return
		}
	}
	// Fallback to task-local storage
	if t.store != nil {
		if storedObj, err := t.store.Get(obj.URL); err == nil && storedObj != nil {
			*obj = *storedObj
		}
	}
}

func (t *TktubeTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	obj.Status = status
	var storeErr error
	if t.store != nil {
		storeErr = t.store.Update(obj)
	}
	// Update shared registry as well
	if t.shared != nil {
		_ = t.shared.Update(obj)
	}
	t.objects = upsertRuntimeObject(t.objects, obj)
	t.knownURLs = rememberRuntimeURLs(t.objects)
	return storeErr
}

func (t *TktubeTask) GetAllObjects() []*model.DownloadObject {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.objects
}

func (t *TktubeTask) snapshotRuntimeObjects() []*model.DownloadObject {
	return append([]*model.DownloadObject(nil), t.objects...)
}

func (t *TktubeTask) rememberRuntimeObject(obj *model.DownloadObject) {
	if obj == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.objects = upsertRuntimeObject(t.objects, obj)
	t.knownURLs = rememberRuntimeURLs(t.objects)
}

// GetConcurrency returns the configured concurrency limit for this task
func (t *TktubeTask) GetConcurrency() int {
	return t.concurrency
}

func (t *TktubeTask) GetRefreshInterval() int {
	return t.refreshInt
}

func (t *TktubeTask) SetConcurrency(n int) error {
	if n < 0 || n > 100 {
		return fmt.Errorf("concurrency must be >= 0 and <= 100")
	}
	t.mu.Lock()
	t.concurrency = n
	t.mu.Unlock()
	return nil
}

func (t *TktubeTask) SetRefreshInterval(sec int) error {
	if sec < 10 || sec > 86400 {
		return fmt.Errorf("refresh interval must be >= 10 and <= 86400")
	}
	t.mu.Lock()
	t.refreshInt = sec
	t.mu.Unlock()
	if t.refresher != nil {
		t.refresher.UpdateInterval(sec)
	}
	return nil
}

// SetObjectIndex moves an object to a new position (for reordering)
func (t *TktubeTask) SetObjectIndex(url string, newIndex int) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	currentIndex := -1
	for i, obj := range t.objects {
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
	if newIndex >= len(t.objects) {
		newIndex = len(t.objects) - 1
	}

	if currentIndex == newIndex {
		return nil
	}

	obj := t.objects[currentIndex]

	// Remove
	t.objects = append(t.objects[:currentIndex], t.objects[currentIndex+1:]...)

	// Insert
	t.objects = append(t.objects[:newIndex], append([]*model.DownloadObject{obj}, t.objects[newIndex:]...)...)

	return nil
}

// --- Helpers ---

func (t *TktubeTask) buildPageURL(page int) string {
	ts := time.Now().UnixMilli()
	switch t.taskType {
	case "tag":
		return fmt.Sprintf("http://129.226.212.209:18080/tktube.com/tags/%s/?mode=async&function=get_block&block_id=list_videos_common_videos_list&sort_by=post_date&from=%d&_=%d", t.keyword, page, ts)
	case "model":
		return fmt.Sprintf("http://129.226.212.209:18080/tktube.com/models/%s/?mode=async&function=get_block&block_id=list_videos_common_videos_list&sort_by=post_date&from=%d&_=%d", t.keyword, page, ts)
	case "search":
		return fmt.Sprintf("http://129.226.212.209:18080/tktube.com/zh/search/?q=%s&mode=async&function=get_block&block_id=list_videos_videos_list_search_result&category_ids=&sort_by=post_date&from_videos=%d&from_albums=%d&_=%d", t.keyword, page, page, ts)
	default:
		return ""
	}
}

func (t *TktubeTask) runScraper(url string) (string, error) {
	return downloader.Scrape(url, "")
}

type videoItem struct {
	href       string
	title      string
	previewURL string
	thumbURL   string
	duration   string
	date       string
}

func videoItemURLs(items []videoItem) []string {
	urls := make([]string, 0, len(items))
	for _, item := range items {
		if item.href == "" {
			continue
		}
		urls = append(urls, item.href)
	}
	return urls
}

func (t *TktubeTask) parseHomePage(html string) ([]videoItem, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var items []videoItem
	doc.Find(".list-videos>.margin-fix>.item").Each(func(i int, s *goquery.Selection) {
		a := s.Find("a")
		href, exists := a.Attr("href")
		if !exists {
			return
		}
		href = strings.TrimSpace(href)
		// Normalize relative hrefs to absolute
		if strings.HasPrefix(href, "/") {
			href = "https://tktube.com" + href
		}
		// Basic sanity check to avoid corrupted values
		if !strings.HasPrefix(href, "http") {
			return
		}

		var title string
		s.Find(".title").First().Each(func(i int, s *goquery.Selection) {
			title = strings.TrimSpace(s.Text())
			title = strings.ReplaceAll(title, "/", "／")
			title = strings.TrimRight(title, ".")
		})

		// Preview URL
		previewURL := ""
		img := s.Find(".img img.thumb")
		if val, exists := img.Attr("data-preview"); exists {
			previewURL = val
		}

		// Thumb URL
		thumbURL, _ := img.Attr("src")

		// Duration
		duration := strings.TrimSpace(s.Find(".duration").Text())

		// Date
		date := strings.TrimSpace(s.Find(".added em").Text())

		items = append(items, videoItem{
			href:       href,
			title:      title,
			previewURL: previewURL,
			thumbURL:   thumbURL,
			duration:   duration,
			date:       date,
		})
	})
	return items, nil
}

var ErrNoFlashvars = fmt.Errorf("flashvars script not found")

type detailedVideoInfo struct {
	title    string
	tags     []string
	videoURL string
	imageURL string
}

func (t *TktubeTask) parseVideoPage(pageURL string) (*detailedVideoInfo, error) {
	html, err := t.runScraper(pageURL)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	info := &detailedVideoInfo{}

	// Title
	info.title = strings.TrimSpace(doc.Find("h1").Text())
	info.title = strings.ReplaceAll(info.title, "/", "／")
	info.title = strings.TrimRight(info.title, ".")

	// Info items
	doc.Find(".info>.item").Each(func(i int, s *goquery.Selection) {
		if i == 1 {
			// Title info
		} else if i == 3 {
			s.Find("a").Each(func(_ int, tag *goquery.Selection) {
				info.tags = append(info.tags, tag.Text())
			})
		}
	})

	// JS Extraction
	scriptContent := ""

	// Try finding the specific script (nth-child(3))
	// goquery uses 0-based index for Eq.
	// .player>.player-holder script
	playerScripts := doc.Find(".player>.player-holder script")
	if playerScripts.Length() >= 3 {
		scriptContent = playerScripts.Eq(2).Text()
	}

	// Fallback: search for flashvars
	if !strings.Contains(scriptContent, "flashvars") {
		doc.Find("script").Each(func(i int, s *goquery.Selection) {
			if strings.Contains(s.Text(), "var flashvars = {") {
				scriptContent = s.Text()
			}
		})
	}

	if scriptContent == "" {
		return nil, ErrNoFlashvars
	}

	// Setup JS VM
	vm := goja.New()

	vm.Set("window", map[string]any{
		"parseInt": func(s string) int64 {
			return 0
		},
	})

	// Load player_util.js
	_, err = vm.RunString(playerUtilJS)
	if err != nil {
		return nil, fmt.Errorf("failed to load player_util.js: %v", err)
	}

	// Extract and run flashvars definition
	start := strings.Index(scriptContent, "var flashvars = {")
	if start == -1 {
		return nil, fmt.Errorf("flashvars definition not found")
	}

	rest := scriptContent[start:]
	end := strings.Index(rest, "};")
	if end != -1 {
		flashvarsDef := rest[:end+2]
		_, err = vm.RunString(flashvarsDef)
		if err != nil {
			return nil, fmt.Errorf("failed to run flashvars definition: %v", err)
		}
	} else {
		// Fallback: try to find the matching brace properly if nested?
		// For now, assume it ends with }; as per standard pattern
		return nil, fmt.Errorf("could not find end of flashvars definition")
	}

	// Run main()
	val, err := vm.RunString("main()")
	if err != nil {
		return nil, fmt.Errorf("failed to run main(): %v", err)
	}

	resultExport := val.Export()
	resultArray, ok := resultExport.([]any)
	if !ok {
		return nil, fmt.Errorf("main() result is not an array")
	}

	if len(resultArray) < 7 {
		return nil, fmt.Errorf("main() result too short")
	}

	info.videoURL = resultArray[0].(string)
	info.imageURL = resultArray[6].(string)

	return info, nil
}

// playerUtilJS content (cleaned up and bX added)
const playerUtilJS = `
var flashvars = {};

function bX(a) {
	return ""; // Dummy implementation to prevent crash if missing
}

function step1(a, b, c, d, e) {
    for (var f in a)
        if (0 == a[f].indexOf(b)) {
            var g = a[f].substring(b.length).split(b[b.length - 1]);

            var h = g[6].substring(0, 2 * parseInt(d)),
                i = e ? e(a, c, d) : "";

            if (i && h) {
                for (var j = h, k = h.length - 1; k >= 0; k--) {
                    for (var l = k, m = k; m < i.length; m++)
                        l += parseInt(i[m]);
                    for (; l >= h.length;)
                        l -= h.length;
                    for (var n = "", o = 0; o < h.length; o++)
                        n += o == k ? h[l] : o == l ? h[k] : h[o];
                    h = n
                }
                g[6] = g[6].replace(j, h),
                    g.splice(0, 1),
                    a[f] = g.join(b[b.length - 1])
            }
        }
}

function step2(a, b, c) {
    var e, g, h, i, j, k, l, m, n, d = "",
        f = "",
        o = parseInt; 
    for (e in a)
        if (e.indexOf(b) > 0 && a[e].length == o(c)) {
            d = a[e];
            break
        }
    if (d) {
        for (f = "",
            g = 1; g < d.length; g++)
            f += o(d[g]) ? o(d[g]) : 1;
        for (j = o(f.length / 2),
            k = o(f.substring(0, j + 1)),
            l = o(f.substring(j)),
            g = l - k,
            g < 0 && (g = -g),
            f = g,
            g = k - l,
            g < 0 && (g = -g),
            f += g,
            f *= 2,
            f = "" + f,
            i = o(c) / 2 + 2,
            m = "",
            g = 0; g < j + 1; g++)
            for (h = 1; h <= 4; h++)
                n = o(d[g + h]) + o(f[g]),
                n >= i && (n -= i),
                m += n;
        return m
    }
    return d
}

function b$() {
    return (new Date).getTime()
}

function cm() {
    var a = Array.prototype.slice.call(arguments);
    return a.join(bX(2))
}

function get_list(a) {
    var z = [];
    if (!!a) {
        var b, c = 'video_url',
            d, e, f, g = !1,
            h = parseInt(a['default_slot']) || 1,
            i, j;
        f = '720p';
        a['skip_selected_format'] == 'true' && (f = null);
        a['rnd'] = b$();
        for (b = 0; b <= 7; b++)
            b > 0 && (c = 'video_alt_url',
                b > 1 && (c += b)),
            a[c] && (d = a[c],
                e = [
                    d,
                    d.toLowerCase().indexOf('.flv') > 0 ? 'video/flash' : 'video/mp4',
                    a[c + '_text'] || '',
                    a[c + '_redirect'] || 0,
                    (a[c + '_4k'] ? 2 : a[c + '_hd'] ? 1 : 0) || 0,
                    f ? f == a[c + '_text'] : !1,
                    a['preview_url'],
                ],
                i && (e[0] = cm(d, d.indexOf('?') >= 0 ? '&' : '?', 'rnd=', a['rnd'])),
                z.push(e),
                e[5] && (g = !0,
                    e[3] && (e[5] = !1,
                        g = !1)));
    }
    return z;
}

function main() {
    step1(flashvars, 'function/', 'code', "16px", step2);
    list = get_list(flashvars);
    return list[list.length - 1]
}
`
