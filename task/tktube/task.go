// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tktube

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/downloader"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/configutil"
	"github.com/cocomhub/download-manager/pkg/dlcore"
	"github.com/cocomhub/download-manager/pkg/titlegroup"
	"github.com/cocomhub/download-manager/task"

	"github.com/PuerkitoBio/goquery"
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
	taskType      string // "tag", "model", "search"
	keyword       string
	pageStart     int
	pageEnd       int
	prefetchQueue chan *model.DownloadObject
	prefetchRate  int64

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
		BaseTask:      bt,
		taskType:      subtype,
		keyword:       keyword,
		pageStart:     1,
		pageEnd:       1,
		prefetchQueue: make(chan *model.DownloadObject, 100), // Buffer
		prefetchRate:  configutil.GetInt64(extra, "prefetch_rate", 10),
	}
	// Start prefetch workers
	go t.startPrefetchWorkers(3) // 3 parallel prefetchers

	// Create PagingScanner for unified scrape pipeline
	adapter := &tktubeAdapter{t: t}
	scanner := task.NewPagingScanner(bt, adapter)
	bt.SetScanner(scanner)

	return t, nil
}

func (t *Task) Type() string {
	return TaskType
}

func (t *Task) GetDownloadObjects() ([]*model.DownloadObject, error) {
	// Enqueue prefetch for pending objects
	t.WithObjectsLock(func(objs []*model.DownloadObject) {
		for _, obj := range objs {
			if obj.GetStatus() == dlcore.StatusPending {
				_, hasLocalPreview := obj.Extra["local_preview"]
				if !hasLocalPreview {
					t.queuePrefetch(obj)
				}
			}
		}
	})

	runtimeObjects := t.SnapshotRuntimeObjects(true)

	// Return pending objects that are ready for download
	var activeCount int64
	if t.Storage() != nil {
		count, err := t.Storage().Count(&core.StorageQuery{
			Filter: core.StorageFilter{
				TaskIDs:  []string{t.ID()},
				Statuses: []string{dlcore.StatusDownloading},
			},
		})
		if err == nil {
			activeCount = count
		}
	}
	if activeCount == 0 {
		for _, obj := range runtimeObjects {
			if obj.GetStatus() == dlcore.StatusDownloading {
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
		if obj.GetStatus() != dlcore.StatusCompleted && obj.GetStatus() != dlcore.StatusCancelled {
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
					t.UpdateStatus(o, dlcore.StatusFailed, err)
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
func (t *Task) ResolveObject(obj *model.DownloadObject) error {
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

func (t *Task) queuePrefetch(obj *model.DownloadObject) {
	select {
	case t.prefetchQueue <- obj:
	default:
		// Queue full, ignore
	}
}

// --- Prefetch Logic ---

func (t *Task) startPrefetchWorkers(count int) {
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

func (t *Task) prefetchAssets(obj *model.DownloadObject) {
	// Don't prefetch if already completed or downloading main
	if obj.GetStatus() == dlcore.StatusCompleted || obj.GetStatus() == dlcore.StatusDownloading || obj.GetStatus() == dlcore.StatusCancelled {
		return
	}

	// Check if already prefetched (double check)
	var hasPreview, hasCover bool
	t.WithLock(func() {
		_, hasPreview = obj.Extra["local_preview"]
		_, hasCover = obj.Extra["local_cover"]
	})

	if hasPreview && hasCover {
		return
	}

	// 1. Preview Video
	previewURL, _ := obj.Extra["preview_url"].(string)
	if previewURL != "" && !hasPreview {
		baseName := strings.ReplaceAll(obj.Metadata["title"], "/", "_")
		path := filepath.Join(t.SaveDir(), baseName+"_preview.mp4")

		if err := t.downloadFile(previewURL, path); err == nil {
			t.WithLock(func() {
				obj.Extra["local_preview"] = path
			})
			t.FlushObject(obj)
			t.Logger().Debug("Prefetched preview", "title", obj.Metadata["title"])
		} else {
			t.Logger().Warn("Failed to prefetch preview, retrying later", "url", previewURL, "error", err)
			go func() {
				time.Sleep(10 * time.Second)
				t.queuePrefetch(obj)
			}()
			return
		}
	}

	// 2. Cover Image
	thumbURL, _ := obj.Extra["thumb_url"].(string)
	if thumbURL != "" && !hasCover {
		baseName := strings.ReplaceAll(obj.Metadata["title"], "/", "_")
		path := filepath.Join(t.SaveDir(), baseName+"_thumb.jpg")

		if err := t.downloadFile(thumbURL, path); err == nil {
			t.WithLock(func() {
				obj.Extra["local_cover"] = path
			})
			t.FlushObject(obj)
			t.Logger().Debug("Prefetched cover", "title", obj.Metadata["title"])
		} else {
			go func() {
				time.Sleep(10 * time.Second)
				t.queuePrefetch(obj)
			}()
		}
	}
}

func (t *Task) downloadFile(url, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	if t.Downloader() == nil {
		return t.simpleDownload(url, path)
	}

	obj := &model.DownloadObject{
		TaskID:   t.ID(),
		URL:      url,
		SavePath: path,
		Metadata: map[string]string{"type": "image"},
		Extra:    map[string]any{},
		Status:   dlcore.StatusPending,
	}
	return t.Downloader().Download(obj, t.GetDownloadHeaders())
}

func (t *Task) simpleDownload(url, path string) error {
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
		Status: dlcore.StatusPending,
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
	t.Logger().Info("Resolving video details", "title", obj.Metadata["title"])
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
