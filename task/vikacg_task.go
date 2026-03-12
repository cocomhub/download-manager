// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/downloader"
	"github.com/cocomhub/download-manager/model"

	"github.com/PuerkitoBio/goquery"
)

type VikacgTask struct {
	id          string
	saveDir     string
	concurrency int
	objects     []*model.DownloadObject
	store       core.Storage
	shared      core.SharedRegistry
	mu          sync.Mutex
	refreshInt  int
	initialized atomic.Int32
	refresher   *CommonRefresher
	knownURLs   map[string]bool
	userID      int
	hasUpdate   atomic.Bool
	pageCount   int
	authToken   string
	cookie      string
	userAgent   string
}

var _ core.Task = &VikacgTask{}

func NewVikacgTask(cfg config.Task, store core.Storage) (*VikacgTask, error) {
	extra := cfg.Extra
	var urls []string
	if extra != nil {
		if v, ok := extra["urls"]; ok {
			switch vv := v.(type) {
			case []string:
				urls = vv
			case []any:
				for _, it := range vv {
					if s, ok := it.(string); ok && s != "" {
						urls = append(urls, s)
					}
				}
			}
		}
	}
	getString := func(key, def string) string {
		if extra == nil {
			return def
		}
		if v, ok := extra[key].(string); ok {
			return v
		}
		return def
	}
	getInt := func(key string, def int) int {
		if extra == nil {
			return def
		}
		if v, ok := extra[key].(int); ok {
			return v
		}
		if v, ok := extra[key].(float64); ok {
			return int(v)
		}
		return def
	}
	t := &VikacgTask{
		id:          cfg.ID,
		saveDir:     cfg.SaveDir,
		concurrency: getInt("max_concurrent", 2),
		objects:     make([]*model.DownloadObject, 0),
		store:       store,
		refreshInt:  getInt("refresh_interval", 3600),
		knownURLs:   make(map[string]bool),
		pageCount:   getInt("page_count", 24),
		authToken:   getString("auth_token", ""),
		cookie:      getString("cookie", ""),
		userAgent:   getString("user_agent", downloader.DefaultUserAgent),
	}
	for _, u := range urls {
		cached := t.getCachedObject(u)
		if cached != nil {
			cached.TaskID = t.id
			t.sanitizeCachedContentHTML(cached)
			t.objects = append(t.objects, cached)
			continue
		}
		obj, err := t.scrapeAndBuild(u)
		if err != nil {
			slog.Warn("vikacg parse failed", "task_id", cfg.ID, "url", u, "error", err)
			continue
		}
		t.objects = append(t.objects, obj)
	}
	for _, o := range t.objects {
		t.knownURLs[o.URL] = true
	}
	t.userID = getInt("user_id", 0)
	if t.userID > 0 {
		t.refresher = NewCommonRefresher(t.refreshInt)
		t.refresher.Start(t.refreshLatestUserPosts)
	}
	return t, nil
}

func (t *VikacgTask) SetSharedRegistry(reg core.SharedRegistry) {
	t.shared = reg
}

func (t *VikacgTask) ID() string {
	return t.id
}

func (t *VikacgTask) Type() string {
	return "vikacg"
}

func (t *VikacgTask) Close() error {
	if t.store != nil {
		if flusher, ok := t.store.(interface{ ForceFlush() error }); ok {
			return flusher.ForceFlush()
		}
	}
	if t.refresher != nil {
		t.refresher.Stop()
	}
	return nil
}

func (t *VikacgTask) GetDownloadHeaders() map[string]string {
	return map[string]string{
		"Cookie":     t.cookie,
		"User-Agent": t.userAgent,
	}
}

func (t *VikacgTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	if t.userID > 0 {
		if t.initialized.CompareAndSwap(0, -1) {
			go t.scrapeUserAllPages()
			return []*model.DownloadObject{}, nil
		}
		if t.initialized.Load() != 1 {
			return []*model.DownloadObject{}, nil
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.shared != nil {
		for _, obj := range t.objects {
			if so, err := t.shared.Get(obj.URL); err == nil && so != nil {
				*obj = *so
			}
		}
	}
	pending := make([]*model.DownloadObject, 0)
	for _, o := range t.objects {
		if o.Status == model.StatusPending || o.Status == model.StatusFailed {
			pending = append(pending, o)
		}
	}
	return pending, nil
}

func (t *VikacgTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	obj.Status = status
	if t.store != nil {
		if e := t.store.Update(obj); e != nil {
			slog.Error("storage update failed", "task_id", t.id, "error", e)
		}
	}
	if t.shared != nil {
		_ = t.shared.Update(obj)
	}
	t.hasUpdate.Store(true)
	return nil
}

func (t *VikacgTask) GetAllObjects() []*model.DownloadObject {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.objects
}

func (t *VikacgTask) GetConcurrency() int {
	return t.concurrency
}

func (t *VikacgTask) scrapeAndBuild(pageURL string) (*model.DownloadObject, error) {
	html, err := downloader.ScraperNative(pageURL, t.cookie)
	if err != nil {
		return nil, err
	}
	id := strings.TrimPrefix(pageURL, "https://www.vikacg.com/p/")
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(doc.Find("meta[property='og:title']").AttrOr("content", ""))
	if title == "" {
		title = strings.TrimSpace(doc.Find("title").Text())
	}
	title = stripSiteSuffix(title)
	title = strings.ReplaceAll(title, "/", "／")
	title = strings.TrimRight(title, ".")
	title = fmt.Sprintf("[%s] %s", id, title)
	section := strings.TrimSpace(doc.Find("meta[property='article:section']").AttrOr("content", ""))
	date := strings.TrimSpace(doc.Find("meta[property='article:published_time']").AttrOr("content", ""))
	if date == "" {
		date = strings.TrimSpace(doc.Find("meta[property='og:created_time']").AttrOr("content", ""))
	}
	updated := strings.TrimSpace(doc.Find("meta[property='article:modified_time']").AttrOr("content", ""))
	desc := strings.TrimSpace(doc.Find("meta[name='description']").AttrOr("content", ""))
	var tags []string
	doc.Find("meta[property='article:tag']").Each(func(i int, s *goquery.Selection) {
		v := strings.TrimSpace(s.AttrOr("content", ""))
		if v != "" {
			tags = append(tags, v)
		}
	})
	if len(tags) == 0 {
		doc.Find("a[href^='/post/tag/']").Each(func(i int, s *goquery.Selection) {
			v := strings.TrimSpace(s.Text())
			if v != "" {
				tags = append(tags, v)
			}
		})
	}
	images := make([]string, 0, 16)
	doc.Find("img.arco-image-img, img.render-arco-image").Each(func(i int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src != "" {
			images = append(images, src)
		}
	})
	if len(images) == 0 {
		cover := strings.TrimSpace(doc.Find("meta[property='og:image']").AttrOr("content", ""))
		if cover != "" {
			images = append(images, cover)
		}
	}
	images = dedupe(images)
	links := make([]map[string]string, 0, 8)
	doc.Find("a[href^='/external']").Each(func(i int, s *goquery.Selection) {
		h := strings.TrimSpace(s.AttrOr("href", ""))
		txt := strings.TrimSpace(s.Text())
		if h != "" {
			links = append(links, map[string]string{
				"text": txt,
				"href": h,
			})
		}
	})
	contentSel := doc.Find(".prose").First()
	contentSel.Find("script, style, noscript").Remove()
	contentSel.Find("img.arco-image-img, img.render-arco-image").Remove()
	contentSel.Find("img").Each(func(i int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src == "" {
			return
		}
		for _, u := range images {
			if src == strings.TrimSpace(u) {
				s.Remove()
				break
			}
		}
	})
	contentHTML, _ := contentSel.Html()
	contentText := strings.TrimSpace(contentSel.Text())
	if contentText == "" {
		contentText = desc
	}
	savePath := filepath.Join(t.saveDir, sanitize(title))
	files := make([]map[string]string, 0, len(images))
	for i, img := range images {
		ext := path.Ext(img)
		if ext == "" {
			ext = ".jpg"
		}
		name := fmt.Sprintf("%02d%s", i+1, ext)
		p := filepath.Join(savePath, name)
		files = append(files, map[string]string{
			"url":  img,
			"path": p,
			"type": "image",
		})
	}
	tagAny := make([]any, len(tags))
	for i := range tags {
		tagAny[i] = tags[i]
	}
	obj := &model.DownloadObject{
		TaskID:   t.id,
		URL:      pageURL,
		SavePath: savePath,
		Metadata: map[string]string{
			"title":    title,
			"page_url": pageURL,
			"type":     "composite",
			"date":     date,
			"section":  section,
			"updated":  updated,
		},
		Extra: map[string]any{
			"tags":         tagAny,
			"content_text": contentText,
			"content_html": strings.TrimSpace(contentHTML),
			"images":       images,
			"files":        files,
			"links":        links,
		},
		Status: model.StatusPending,
	}
	t.restoreStatus(obj)
	return obj, nil
}

func (t *VikacgTask) restoreStatus(obj *model.DownloadObject) {
	if t.shared != nil {
		if so, err := t.shared.Get(obj.URL); err == nil && so != nil {
			*obj = *so
			return
		}
	}
	if t.store != nil {
		if so, err := t.store.Get(obj.URL); err == nil && so != nil {
			*obj = *so
		}
	}
}

func sanitize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	return s
}

func (t *VikacgTask) getCachedObject(url string) *model.DownloadObject {
	if t.shared != nil {
		if so, err := t.shared.Get(url); err == nil && so != nil {
			return so
		}
	}
	if t.store != nil {
		if so, err := t.store.Get(url); err == nil && so != nil {
			return so
		}
	}
	return nil
}

func (t *VikacgTask) sanitizeCachedContentHTML(obj *model.DownloadObject) {
	if obj == nil || obj.Extra == nil {
		return
	}
	raw, _ := obj.Extra["content_html"].(string)
	if strings.TrimSpace(raw) == "" {
		return
	}
	wrapped := "<div id=\"root\">" + raw + "</div>"
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(wrapped))
	if err != nil {
		return
	}
	imgSet := make(map[string]struct{})
	if filesVal, ok := obj.Extra["files"]; ok {
		if list, ok := filesVal.([]map[string]string); ok {
			for _, f := range list {
				if u := strings.TrimSpace(f["url"]); u != "" {
					imgSet[u] = struct{}{}
				}
			}
		}
		if list, ok := filesVal.([]any); ok {
			for _, it := range list {
				if m, ok := it.(map[string]any); ok {
					if u, ok := m["url"].(string); ok && strings.TrimSpace(u) != "" {
						imgSet[strings.TrimSpace(u)] = struct{}{}
					}
				}
			}
		}
	}
	if imgsVal, ok := obj.Extra["images"]; ok {
		if arr, ok := imgsVal.([]string); ok {
			for _, u := range arr {
				if strings.TrimSpace(u) != "" {
					imgSet[strings.TrimSpace(u)] = struct{}{}
				}
			}
		}
		if arr, ok := imgsVal.([]any); ok {
			for _, it := range arr {
				if u, ok := it.(string); ok && strings.TrimSpace(u) != "" {
					imgSet[strings.TrimSpace(u)] = struct{}{}
				}
			}
		}
	}
	doc.Find("img.arco-image-img, img.render-arco-image").Each(func(i int, s *goquery.Selection) {
		s.Remove()
	})
	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src == "" {
			return
		}
		if _, ok := imgSet[src]; ok {
			s.Remove()
		}
	})
	doc.Find("script, style, noscript").Remove()
	root := doc.Find("#root").First()
	if h, e := root.Html(); e == nil {
		obj.Extra["content_html"] = strings.TrimSpace(h)
	}
}

// 去除常见站点后缀，保留纯标题
func stripSiteSuffix(title string) string {
	t := strings.TrimSpace(title)
	suffixes := []string{
		" - 维咔VikACG[V站]",
		" - 维咔VikACG",
		" - 维咔",
	}
	for _, s := range suffixes {
		if before, ok := strings.CutSuffix(t, s); ok {
			return strings.TrimSpace(before)
		}
	}
	// 兜底：出现“ - 维咔”时按首次出现截断
	if idx := strings.Index(t, " - 维咔"); idx > 0 {
		return strings.TrimSpace(t[:idx])
	}
	return t
}

// 简单去重，保持顺序
func dedupe(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		v := strings.TrimSpace(it)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func (t *VikacgTask) GetStorage() core.Storage {
	return t.store
}

func (t *VikacgTask) GetRefreshInterval() int {
	return t.refreshInt
}

func (t *VikacgTask) SetConcurrency(n int) error {
	if n < 0 || n > 100 {
		return fmt.Errorf("concurrency must be >= 0 and <= 100")
	}
	t.mu.Lock()
	t.concurrency = n
	t.mu.Unlock()
	return nil
}

func (t *VikacgTask) SetRefreshInterval(sec int) error {
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

func (t *VikacgTask) getCachePath() string {
	return filepath.Join(config.GetWorkDir(), "cache", t.id+".json")
}

func (t *VikacgTask) SaveCache() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.hasUpdate.CompareAndSwap(true, false) {
		return nil
	}
	data, err := json.MarshalIndent(t.objects, "", "  ")
	if err != nil {
		t.hasUpdate.Store(true)
		return err
	}
	path := t.getCachePath()
	os.MkdirAll(filepath.Dir(path), 0755)
	return os.WriteFile(path, data, 0644)
}

func (t *VikacgTask) LoadCache() error {
	t.initialized.Store(-1)
	defer func() {
		if len(t.objects) > 0 {
			t.initialized.Store(1)
		} else {
			t.initialized.Store(0)
		}
	}()
	t.mu.Lock()
	defer t.mu.Unlock()
	path := t.getCachePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var objects []*model.DownloadObject
	if err := json.Unmarshal(data, &objects); err != nil {
		return err
	}
	t.objects = objects
	t.knownURLs = make(map[string]bool)
	for _, obj := range t.objects {
		t.knownURLs[obj.URL] = true
		t.restoreStatus(obj)
		t.sanitizeCachedContentHTML(obj)
		if obj.Status != model.StatusCompleted && obj.Status != model.StatusCancelled {
			if obj.Status == model.StatusDownloading {
				obj.Status = model.StatusPending
			}
		}
	}
	return nil
}

type vikPost struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updated_at"`
	CreatedAt string `json:"created_at"`
	Summary   string `json:"summary"`
	Thumb     string `json:"thumb"`
}

type getPostsResp struct {
	Data struct {
		List []vikPost `json:"list"`
	} `json:"data"`
}

func (t *VikacgTask) getPostsPage(page int) ([]vikPost, error) {
	body := map[string]any{
		"order":      "updated_at",
		"sort":       "desc",
		"status":     []string{"publish", "publish_anti"},
		"search":     nil,
		"page_count": t.pageCount,
		"paged":      page,
		"category":   nil,
		"tag":        nil,
		"rating":     nil,
		"is_pinned":  false,
		"user_id":    t.userID,
	}
	data, _ := json.Marshal(body)
	url := "http://129.226.212.209:18080/www.vikacg.com/api/vikacg/v1/getPosts"
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", "https://www.vikacg.com")
	req.Header.Set("Referer", fmt.Sprintf("https://www.vikacg.com/u/%d/post", t.userID))
	req.Header.Set("X-Client-Name", "VikACG Moonlight")
	if t.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+t.authToken)
	}
	if t.cookie != "" {
		req.Header.Set("Cookie", t.cookie)
	}
	if t.userAgent != "" {
		req.Header.Set("User-Agent", t.userAgent)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	slog.Debug("vikacg getPosts request", "task_id", t.id, "page", page, "url", url)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out getPostsResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	slog.Info("vikacg getPosts response", "task_id", t.id, "page", page, "url", url, "posts", len(out.Data.List))
	return out.Data.List, nil
}

func (t *VikacgTask) scrapeUserAllPages() {
	defer t.initialized.Store(1)
	t.mu.Lock()
	startKnown := make(map[string]bool, len(t.knownURLs))
	maps.Copy(startKnown, t.knownURLs)
	t.mu.Unlock()
	page := 1
	for {
		posts, err := t.getPostsPage(page)
		if err != nil {
			slog.Error("vikacg getPosts failed", "task_id", t.id, "page", page, "error", err)
			break
		}
		if len(posts) == 0 {
			break
		}
		newObjs := make([]*model.DownloadObject, 0, len(posts))
		for _, p := range posts {
			u := fmt.Sprintf("https://www.vikacg.com/p/%d", p.ID)
			t.mu.Lock()
			known := t.knownURLs[u]
			t.mu.Unlock()
			if known {
				continue
			}
			if cached := t.getCachedObject(u); cached != nil {
				cached.TaskID = t.id
				t.sanitizeCachedContentHTML(cached)
				newObjs = append(newObjs, cached)
			} else {
				obj, err := t.scrapeAndBuild(u)
				if err != nil {
					slog.Warn("vikacg page parse failed", "task_id", t.id, "url", u, "error", err)
					continue
				}
				newObjs = append(newObjs, obj)
			}
			t.mu.Lock()
			t.knownURLs[u] = true
			t.mu.Unlock()
		}
		if len(newObjs) > 0 {
			t.mu.Lock()
			t.objects = append(newObjs, t.objects...)
			t.mu.Unlock()
			t.hasUpdate.Store(true)
			if err := t.SaveCache(); err != nil {
				slog.Error("vikacg save cache failed", "task_id", t.id, "error", err)
			}
		}
		page++
	}
	slog.Info("vikacg init done", "task_id", t.id, "total_objects", len(t.GetAllObjects()))
}

func (t *VikacgTask) refreshLatestUserPosts() {
	if t.userID <= 0 {
		return
	}
	if t.initialized.Load() != 1 {
		return
	}
	page := 1
	var known bool
	pageObjs := make([]*model.DownloadObject, 0, t.pageCount)
	for !known {
		posts, err := t.getPostsPage(page)
		if err != nil {
			slog.Error("vikacg refresh failed", "task_id", t.id, "page", page, "error", err)
			break
		}
		if len(posts) == 0 {
			break
		}
		for _, p := range posts {
			u := fmt.Sprintf("https://www.vikacg.com/p/%d", p.ID)
			t.mu.Lock()
			known2 := t.knownURLs[u]
			t.mu.Unlock()
			if known2 {
				known = known2
				continue
			}
			if cached := t.getCachedObject(u); cached != nil {
				cached.TaskID = t.id
				t.sanitizeCachedContentHTML(cached)
				pageObjs = append(pageObjs, cached)
			} else {
				obj, err := t.scrapeAndBuild(u)
				if err != nil {
					slog.Warn("vikacg page parse failed", "task_id", t.id, "url", u, "error", err)
					continue
				}
				pageObjs = append(pageObjs, obj)
			}
			t.mu.Lock()
			t.knownURLs[u] = true
			t.mu.Unlock()
		}
	}
	if len(pageObjs) > 0 {
		t.mu.Lock()
		t.objects = append(pageObjs, t.objects...)
		t.mu.Unlock()
		t.hasUpdate.Store(true)
		if err := t.SaveCache(); err != nil {
			slog.Error("vikacg save cache failed after refresh", "task_id", t.id, "error", err)
		}
		slog.Info("vikacg refresh finished", "task_id", t.id, "new_items", len(pageObjs))
	} else {
		slog.Info("vikacg refresh no new", "task_id", t.id)
	}
}
