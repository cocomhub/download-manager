// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package vikacg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/downloader"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/configutil"
	"github.com/cocomhub/download-manager/pkg/logutil"
	"github.com/cocomhub/download-manager/task"
)

const TaskType = "vikacg"

// arcoImageSelector is the CSS selector for Arco Design system images on vikacg.com.
const arcoImageSelector = "img.arco-image-img, img.render-arco-image"

func init() {
	task.Register(TaskType, func(cfg *config.Task, opts task.Options) (core.Task, error) {
		return NewTask(cfg, opts)
	})
}

type Task struct {
	*task.BaseTask
	userID    int64
	pageCount int64
	authToken string
	cookie    string
	userAgent string
}

var _ core.Task = &Task{}

func NewTask(cfg *config.Task, opts task.Options) (*Task, error) {
	urls := extractURLs(cfg.Extra)

	bt, err := task.NewBaseTask(cfg, opts)
	if err != nil {
		return nil, err
	}
	t := &Task{
		BaseTask:  bt,
		userID:    configutil.GetInt64(cfg.Extra, "user_id", 0),
		pageCount: configutil.GetInt64(cfg.Extra, "page_count", 24),
		authToken: configutil.GetString(cfg.Extra, "auth_token", ""),
		cookie:    configutil.GetString(cfg.Extra, "cookie", ""),
		userAgent: configutil.GetString(cfg.Extra, "user_agent", downloader.DefaultUserAgent),
	}

	for _, u := range urls {
		cached := t.GetCachedObject(u)
		if cached != nil {
			cached.TaskID = t.ID()
			t.sanitizeCachedContentHTML(cached)
			t.RememberRuntimeObject(cached, true)
			continue
		}
		obj, err := t.scrapeAndBuild(u)
		if err != nil {
			t.Logger().Warn("vikacg parse failed", logutil.LogKeyURL, u, logutil.LogKeyError, err)
			continue
		}
		t.PersistTaskObject(obj)
		t.RememberRuntimeObject(obj, true)
	}

	_ = t.userID // referenced by Scrape/PagingScanner at runtime

	// Create PagingScanner for unified scrape pipeline
	adapter := &vikacgAdapter{t: t}
	scanner := task.NewPagingScanner(bt, adapter)
	bt.SetScanner(scanner)

	return t, nil
}

func (t *Task) Type() string {
	return TaskType
}

func (t *Task) GetDownloadHeaders() map[string]string {
	return map[string]string{
		"Cookie":     t.cookie,
		"User-Agent": t.userAgent,
	}
}

func (t *Task) GetDownloadObjects() ([]*model.DownloadObject, error) {
	objects := t.LoadPendingFromStorage(64)
	if objects == nil {
		objects = t.SnapshotRuntimeObjects(true)
	}
	pending := make([]*model.DownloadObject, 0)
	for _, o := range objects {
		t.SyncSharedToObject(o)
		if t.IsMarkedFailed(o.URL) {
			continue
		}
		if o.GetStatus() == model.StatusPending || o.GetStatus() == model.StatusFailed || o.GetStatus() == model.StatusDownloading {
			pending = append(pending, o)
		}
	}
	return pending, nil
}

// ResolveObject resolves a vikacg object by re-scraping its detail page.
// It populates the object with fresh content (images, title, metadata, files).
func (t *Task) ResolveObject(_ context.Context, obj *model.DownloadObject) error {
	newObj, err := t.scrapeAndBuild(obj.URL)
	if err != nil {
		return err
	}
	// Copy resolved data into the existing object (not replace it)
	obj.Lock()
	obj.SavePath = newObj.SavePath
	obj.Metadata = newObj.Metadata
	obj.Extra = newObj.Extra
	obj.Unlock()
	return nil
}

func (t *Task) scrapeAndBuild(pageURL string) (*model.DownloadObject, error) {
	html, err := downloader.ScraperNative(pageURL, t.cookie)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	title := t.extractTitle(doc, pageURL, html)
	if title == "" {
		return nil, fmt.Errorf("title is empty")
	}

	section := strings.TrimSpace(doc.Find("meta[property='article:section']").AttrOr("content", ""))
	date := extractDate(doc)
	updated := strings.TrimSpace(doc.Find("meta[property='article:modified_time']").AttrOr("content", ""))
	desc := strings.TrimSpace(doc.Find("meta[name='description']").AttrOr("content", ""))
	tags := extractTags(doc)
	images := extractImages(doc)
	links := extractLinks(doc)
	contentHTML, contentText := cleanContent(doc, desc, images)

	savePath := filepath.Join(t.SaveDir(), sanitize(title))
	files := buildFiles(images, savePath)

	tagAny := make([]any, len(tags))
	for i := range tags {
		tagAny[i] = tags[i]
	}

	obj := &model.DownloadObject{
		TaskID:   t.ID(),
		URL:      pageURL,
		SavePath: savePath,
		Metadata: map[string]string{
			"title":   title,
			"type":    "composite",
			"date":    date,
			"section": section,
			"updated": updated,
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
	t.CheckRestoreCompleted(obj)
	return obj, nil
}

// extractTitle extracts and formats the title from the document.
func (t *Task) extractTitle(doc *goquery.Document, pageURL, html string) string {
	id := strings.TrimPrefix(pageURL, "https://www.vikacg.com/p/")
	title := strings.TrimSpace(doc.Find("meta[property='og:title']").AttrOr("content", ""))
	if title == "" {
		title = strings.TrimSpace(doc.Find("title").Text())
	}
	if title == "" {
		os.WriteFile(fmt.Sprintf("%s.html", id), []byte(html), 0644)
		return ""
	}
	title = stripSiteSuffix(title)
	title = strings.ReplaceAll(title, "/", "／")
	title = strings.TrimRight(title, ".")
	return fmt.Sprintf("[%s] %s", id, title)
}

// extractDate extracts the publication date from the document.
func extractDate(doc *goquery.Document) string {
	date := strings.TrimSpace(doc.Find("meta[property='article:published_time']").AttrOr("content", ""))
	if date == "" {
		date = strings.TrimSpace(doc.Find("meta[property='og:created_time']").AttrOr("content", ""))
	}
	return date
}

// extractTags extracts tags from the document with fallback selectors.
func extractTags(doc *goquery.Document) []string {
	var tags []string
	doc.Find("meta[property='article:tag']").Each(func(i int, s *goquery.Selection) {
		v := strings.TrimSpace(s.AttrOr("content", ""))
		if v != "" {
			tags = append(tags, v)
		}
	})
	if len(tags) > 0 {
		return tags
	}
	doc.Find("a[href^='/post/tag/']").Each(func(i int, s *goquery.Selection) {
		v := strings.TrimSpace(s.Text())
		if v != "" {
			tags = append(tags, v)
		}
	})
	return tags
}

// extractImages extracts image URLs from the document with fallback.
func extractImages(doc *goquery.Document) []string {
	images := make([]string, 0, 16)
	doc.Find(arcoImageSelector).Each(func(i int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src != "" {
			images = append(images, src)
		}
	})
	if len(images) > 0 {
		return dedupe(images)
	}
	cover := strings.TrimSpace(doc.Find("meta[property='og:image']").AttrOr("content", ""))
	if cover != "" {
		images = append(images, cover)
	}
	return dedupe(images)
}

// extractLinks extracts external links from the document.
func extractLinks(doc *goquery.Document) []map[string]string {
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
	return links
}

// cleanContent extracts content HTML and text, removing images and unwanted elements.
func cleanContent(doc *goquery.Document, desc string, images []string) (contentHTML, contentText string) {
	contentSel := doc.Find(".prose").First()
	contentSel.Find("script, style, noscript").Remove()
	contentSel.Find(arcoImageSelector).Remove()
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
	contentHTML, _ = contentSel.Html()
	contentText = strings.TrimSpace(contentSel.Text())
	if contentText == "" {
		contentText = desc
	}
	return
}

// buildFiles creates file entries from image URLs.
func buildFiles(images []string, savePath string) []map[string]string {
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
	return files
}

func sanitize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	return s
}

// collectFileURLs extracts URL strings from the "files" extra field,
// accepting both []map[string]string and []any (from JSON unmarshaling).
// extractURLs extracts URL strings from the "urls" extra field,
// supporting both []string and []any (from JSON unmarshaling).
func extractURLs(extra map[string]any) []string {
	if extra == nil {
		return nil
	}
	v, ok := extra["urls"]
	if !ok {
		return nil
	}
	switch vv := v.(type) {
	case []string:
		return vv
	case []any:
		urls := make([]string, 0, len(vv))
		for _, it := range vv {
			if s, ok := it.(string); ok && s != "" {
				urls = append(urls, s)
			}
		}
		return urls
	}
	return nil
}

func collectFileURLs(extra map[string]any, set map[string]struct{}) {
	filesVal, ok := extra["files"]
	if !ok {
		return
	}
	if list, ok := filesVal.([]map[string]string); ok {
		for _, f := range list {
			if u := strings.TrimSpace(f["url"]); u != "" {
				set[u] = struct{}{}
			}
		}
	}
	if list, ok := filesVal.([]any); ok {
		for _, it := range list {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			u, ok := m["url"].(string)
			if !ok || strings.TrimSpace(u) == "" {
				continue
			}
			set[strings.TrimSpace(u)] = struct{}{}
		}
	}
}

// collectImageURLs extracts URL strings from the "images" extra field,
// accepting both []string and []any (from JSON unmarshaling).
func collectImageURLs(extra map[string]any, set map[string]struct{}) {
	imgsVal, ok := extra["images"]
	if !ok {
		return
	}
	if arr, ok := imgsVal.([]string); ok {
		for _, u := range arr {
			if u = strings.TrimSpace(u); u != "" {
				set[u] = struct{}{}
			}
		}
	}
	if arr, ok := imgsVal.([]any); ok {
		for _, it := range arr {
			u, ok := it.(string)
			if !ok || strings.TrimSpace(u) == "" {
				continue
			}
			set[strings.TrimSpace(u)] = struct{}{}
		}
	}
}

func (t *Task) sanitizeCachedContentHTML(obj *model.DownloadObject) {
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
	collectFileURLs(obj.Extra, imgSet)
	collectImageURLs(obj.Extra, imgSet)
	doc.Find(arcoImageSelector).Each(func(i int, s *goquery.Selection) {
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

// stripSiteSuffix 去除常见站点后缀，保留纯标题
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
	// 兜底：出现 " - 维咔" 时按首次出现截断
	if idx := strings.Index(t, " - 维咔"); idx > 0 {
		return strings.TrimSpace(t[:idx])
	}
	return t
}

// dedupe 简单去重，保持顺序
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

func (t *Task) getPostsPage(ctx context.Context, page int) ([]vikPost, error) {
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
	scraperURL := config.GetServerConfig().ScraperURL
	if scraperURL == "" {
		scraperURL = "http://localhost:18082"
	}
	url := scraperURL + "/www.vikacg.com/api/vikacg/v1/getPosts"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
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
	t.Logger().Debug("vikacg getPosts request", "page", page, logutil.LogKeyURL, url)
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
	t.Logger().Info("vikacg getPosts response", "page", page, logutil.LogKeyURL, url, "posts", len(out.Data.List))
	return out.Data.List, nil
}

func (t *Task) Scrape(ctx context.Context) error {
	if t.userID <= 0 {
		return nil
	}
	// Delegate to BaseTask.Scrape which uses PagingScanner (set in NewTask).
	return t.BaseTask.Scrape(ctx)
}

func vikPostURLs(posts []vikPost) []string {
	urls := make([]string, 0, len(posts))
	for _, p := range posts {
		if p.ID <= 0 {
			continue
		}
		urls = append(urls, fmt.Sprintf("https://www.vikacg.com/p/%d", p.ID))
	}
	return urls
}
