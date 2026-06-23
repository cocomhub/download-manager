// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tktube

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/downloader"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/configutil"
	"github.com/cocomhub/download-manager/pkg/titlegroup"
	"github.com/cocomhub/download-manager/task"
	"github.com/dop251/goja"
)

const TaskType = "tktube"

func init() {
	task.Register(TaskType, func(cfg *config.Task, opts task.Options) (core.Task, error) {
		return NewTask(cfg, opts)
	})
}

// Task implements core.Task for Tktube
type Task struct {
	*task.BaseTask
	taskType  string // "tag", "model", "search"
	keyword   string
	pageStart int
	pageEnd   int

	resolvedURLs sync.Map
}

// Ensure Task implements core.Task
var _ core.Task = (*Task)(nil)

func NewTask(cfg *config.Task, opts task.Options) (*Task, error) {
	extra := cfg.Extra
	if extra == nil {
		extra = make(map[string]any)
	}

	subtype := configutil.GetString(extra, "subtype", "tag")
	keyword := configutil.GetString(extra, "keyword", "")
	if configutil.GetBool(extra, "save_dir_add_keyword", false) {
		cfg.SaveDir = filepath.Join(cfg.SaveDir, keyword)
	}

	bt, err := task.NewBaseTask(cfg, opts)
	if err != nil {
		return nil, err
	}
	t := &Task{
		BaseTask:  bt,
		taskType:  subtype,
		keyword:   keyword,
		pageStart: 1,
		pageEnd:   1,
	}

	// Create PagingScanner for unified scrape pipeline
	adapter := &tktubeAdapter{t: t}
	scanner := task.NewPagingScanner(bt, adapter)
	bt.SetScanner(scanner)

	return t, nil
}

func (t *Task) Type() string {
	return TaskType
}

func (t *Task) Close() error {
	return t.BaseTask.Close()
}

func (t *Task) GetDownloadObjects() ([]*model.DownloadObject, error) {
	runtimeObjects := t.SnapshotRuntimeObjects(true)
	var activeCount int64
	if t.Storage() != nil {
		count, err := t.Storage().Count(&core.StorageQuery{
			Filter: core.StorageFilter{
				TaskIDs:  []string{t.ID()},
				Statuses: []string{model.StatusDownloading},
			},
		})
		if err == nil {
			activeCount = count
		}
	}
	if activeCount == 0 {
		for _, obj := range runtimeObjects {
			if obj.GetStatus() == model.StatusDownloading {
				activeCount++
			}
		}
	}

	candidates := make([]*model.DownloadObject, 0)
	toResolve := make([]*model.DownloadObject, 0)

	queryLimit := int64(max(t.Concurrency()*3+8, 16))
	objects := runtimeObjects
	if stored := t.LoadPendingFromStorage(queryLimit); stored != nil {
		objects = stored
	}

	// Collect candidates
	for _, obj := range objects {
		if obj.GetStatus() != model.StatusCompleted && obj.GetStatus() != model.StatusCancelled {
			if t.IsMarkedFailed(obj.URL) {
				continue
			}
			if int64(len(candidates)+len(toResolve))+activeCount < int64(t.Concurrency()*2+2) {
				_, ok := t.resolvedURLs.Load(obj.URL)
				if _, hasFiles := obj.Extra["files"]; hasFiles && ok {
					candidates = append(candidates, obj)
				} else {
					toResolve = append(toResolve, obj)
				}
			} else {
				break
			}
		}
	}

	// Resolve in parallel
	if len(toResolve) > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex
		sem := make(chan struct{}, 5)

		for _, obj := range toResolve {
			wg.Add(1)
			go func(o *model.DownloadObject) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				if err := t.resolveVideoDetails(o); err != nil {
					t.Logger().Error("Failed to resolve video details", "url", o.URL, "error", err)
					// resolveVideoDetails already calls MarkAsFailed for ErrNoFlashvars
					// (which sets failed_permanent). Don't overwrite with generic StatusFailed.
					if !errors.Is(err, ErrNoFlashvars) {
						t.UpdateStatus(o, model.StatusFailed, err)
					}
				} else {
					mu.Lock()
					t.resolvedURLs.Store(o.URL, true)
					candidates = append(candidates, o)
					mu.Unlock()
				}
			}(obj)
		}
		wg.Wait()
	}

	return candidates, nil
}

// ResolveObject explicitly resolves an object (exposed for Manager)
func (t *Task) ResolveObject(ctx context.Context, obj *model.DownloadObject) error {
	// Check shared state for resolved files first
	if so := t.GetSharedObject(obj.URL); so != nil {
		if files, ok := so.Extra["files"]; ok {
			t.WithLock(func() {
				obj.Extra["files"] = files
			})
			return nil
		}
	}
	// Check if already resolved
	if _, hasFiles := obj.Extra["files"]; hasFiles {
		return nil
	}
	return t.resolveVideoDetails(obj)
}

// --- Scraping Logic ---

func (t *Task) parseTotalPages(html string) int {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return 0
	}

	// Find .pagination .last
	var lastPage int
	doc.Find(".pagination .last a").Each(func(i int, s *goquery.Selection) {
		params, exists := s.Attr("data-parameters")
		if exists {
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

// SmallObjects implements core.SmallObjectCap.
// 返回与主对象关联的小对象（preview 视频 + cover 缩略图）。
func (t *Task) SmallObjects(obj *model.DownloadObject) []core.SmallObjectInfo {
	if obj == nil || obj.Extra == nil || obj.Metadata == nil {
		return nil
	}

	var items []core.SmallObjectInfo

	// Preview 视频
	if previewURL, ok := obj.Extra["preview_url"].(string); ok && previewURL != "" {
		baseName := strings.ReplaceAll(obj.Metadata[model.MetadataKeyTitle], "/", "_")
		path := filepath.Join(t.SaveDir(), baseName+"_preview.mp4")
		items = append(items, core.SmallObjectInfo{
			URL:      previewURL,
			SavePath: path,
			Rel:      "preview",
		})
	}

	// Cover 缩略图
	if thumbURL, ok := obj.Extra["thumb_url"].(string); ok && thumbURL != "" {
		baseName := strings.ReplaceAll(obj.Metadata[model.MetadataKeyTitle], "/", "_")
		path := filepath.Join(t.SaveDir(), baseName+"_thumb.jpg")
		items = append(items, core.SmallObjectInfo{
			URL:      thumbURL,
			SavePath: path,
			Rel:      "cover",
		})
	}

	return items
}

func (t *Task) createObjectFromVideoItem(v videoItem) *model.DownloadObject {
	// Basic object with metadata from list page
	videoPath, _ := t.ResolvePath(v.title, "video")
	group := titlegroup.TKTContentGroupKey(v.title, v.href)

	obj := &model.DownloadObject{
		TaskID:   t.ID(),
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
		Status: model.StatusPending,
	}

	// Deduplication / Restore Status from DB
	t.CheckAndRestoreStatus(obj)

	return obj
}

// SetObjectIndex moves an object to a new position (for reordering)
func (t *Task) SetObjectIndex(url string, newIndex int) error {
	return t.MoveObject(url, newIndex)
}

func (t *Task) resolveVideoDetails(obj *model.DownloadObject) error {
	t.Logger().Info("Resolving video details", "title", obj.Metadata[model.MetadataKeyTitle])
	videoInfo, err := t.parseVideoPage(obj.URL)
	if err != nil {
		if err == ErrNoFlashvars {
			t.MarkAsFailed(obj, err)
			return err
		}
		return err
	}

	videoPath, imagePath := t.ResolvePath(videoInfo.title, "video")

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

	t.WithLock(func() {
		obj.Extra["tags"] = videoInfo.tags
		if _, ok := obj.Extra["files"]; !ok {
			obj.Extra["files"] = files
		}
	})

	// Update storage and shared registry
	t.FlushObject(obj)

	return nil
}

// --- Helpers ---

func (t *Task) buildPageURL(page int) string {
	ts := time.Now().UnixMilli()
	switch t.taskType {
	case "tag":
		return fmt.Sprintf("http://tktube.com/tags/%s/?mode=async&function=get_block&block_id=list_videos_common_videos_list&sort_by=post_date&from=%d&_=%d", t.keyword, page, ts)
	case "model":
		return fmt.Sprintf("http://tktube.com/models/%s/?mode=async&function=get_block&block_id=list_videos_common_videos_list&sort_by=post_date&from=%d&_=%d", t.keyword, page, ts)
	case "search":
		return fmt.Sprintf("http://tktube.com/zh/search/?q=%s&mode=async&function=get_block&block_id=list_videos_videos_list_search_result&category_ids=&sort_by=post_date&from_videos=%d&from_albums=%d&_=%d", t.keyword, page, page, ts)
	default:
		return ""
	}
}

func (t *Task) runScraper(url string) (string, error) {
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

func (t *Task) parseHomePage(html string) ([]videoItem, error) {
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
			title = strings.ReplaceAll(title, "/", "_")
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

func (t *Task) parseVideoPage(pageURL string) (*detailedVideoInfo, error) {
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
	info.title = strings.ReplaceAll(info.title, "/", "_")
	info.title = strings.TrimRight(info.title, ".")

	// Info items
	doc.Find(".info>.item").Each(func(i int, s *goquery.Selection) {
		switch i {
		case 1:
			// Title info
		case 3:
			s.Find("a").Each(func(_ int, tag *goquery.Selection) {
				info.tags = append(info.tags, tag.Text())
			})
		}
	})

	// JS Extraction
	scriptContent := ""

	// Try finding the specific script (nth-child(3))
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
	_, err = vm.RunString(PlayerUtilJS)
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
