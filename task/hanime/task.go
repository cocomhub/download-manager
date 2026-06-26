// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package hanime

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
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

// baseURL is the root URL for hanime1.me API requests.
const baseURL = "https://hanime1.me"

// titleWithVidFmt formats a title with video ID in brackets: [vid] title.
const titleWithVidFmt = "[%s] %s"

const (
	TaskType = "hanime"
)

func init() {
	task.Register(TaskType, func(cfg *config.Task, opts task.Options) (core.Task, error) {
		return NewTask(cfg, opts)
	})
}

type Task struct {
	*task.BaseTask
	genre  string
	cookie string
}

// Ensure Task implements core.Task
var _ core.Task = (*Task)(nil)

func NewTask(cfg *config.Task, opts task.Options) (*Task, error) {
	extra := cfg.Extra
	genre := configutil.GetString(extra, "genre", "裏番")
	if configutil.GetBool(extra, "save_dir_add_genre", false) && genre != "" {
		cfg.SaveDir = filepath.Join(cfg.SaveDir, genre)
	}

	bt, err := task.NewBaseTask(cfg, opts)
	if err != nil {
		return nil, err
	}
	t := &Task{
		BaseTask: bt,
		genre:    genre,
		cookie:   configutil.GetString(extra, "cookie", ""),
	}

	// Create PagingScanner for unified scrape pipeline
	adapter := &hanimeAdapter{t: t}
	scanner := task.NewPagingScanner(bt, adapter)
	bt.SetScanner(scanner)

	return t, nil
}

func (t *Task) Type() string {
	return TaskType
}

func (t *Task) GetDownloadHeaders() map[string]string {
	return map[string]string{
		"Cookie": t.cookie,
	}
}

func (t *Task) GetDownloadObjects() ([]*model.DownloadObject, error) {
	objects := t.LoadPendingFromStorage(64)
	if objects == nil {
		objects = t.SnapshotRuntimeObjects(true)
	}

	activeCount := t.countActiveDownloading(objects)
	candidates, toResolve := t.separateDownloadCandidates(objects, activeCount)

	if len(toResolve) > 0 {
		candidates = t.resolveDownloadCandidates(candidates, toResolve)
	}

	return candidates, nil
}

// countActiveDownloading counts objects currently in StatusDownloading state.
func (t *Task) countActiveDownloading(objects []*model.DownloadObject) int {
	count := 0
	for _, obj := range objects {
		if obj.GetStatus() == model.StatusDownloading {
			count++
		}
	}
	return count
}

// separateDownloadCandidates splits objects into candidates (already resolved)
// and toResolve (needs resolution), respecting the concurrency capacity limit.
func (t *Task) separateDownloadCandidates(objects []*model.DownloadObject, activeCount int) (candidates, toResolve []*model.DownloadObject) {
	maxToResolve := t.Concurrency()*2 + 2
	candidates = make([]*model.DownloadObject, 0)
	toResolve = make([]*model.DownloadObject, 0)

	for _, obj := range objects {
		if t.IsMarkedFailed(obj.URL) {
			continue
		}
		if obj.GetStatus() == model.StatusCompleted || obj.GetStatus() == model.StatusCancelled {
			continue
		}
		if _, hasFiles := obj.Extra["files"]; hasFiles {
			candidates = append(candidates, obj)
			continue
		}
		if len(candidates)+len(toResolve)+activeCount < maxToResolve {
			toResolve = append(toResolve, obj)
		}
	}
	return
}

// resolveDownloadCandidates resolves each unresolved object and appends
// successful ones to candidates, returning the updated candidates slice.
func (t *Task) resolveDownloadCandidates(candidates, toResolve []*model.DownloadObject) []*model.DownloadObject {
	for _, obj := range toResolve {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := t.ResolveObject(ctx, obj); err == nil {
			cancel()
			candidates = append(candidates, obj)
		} else {
			cancel()
			t.Logger().Error("hanime resolve object failed", logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
			t.UpdateStatus(obj, model.StatusFailed, err)
		}
	}
	return candidates
}

type hanimeItem struct {
	href     string
	title    string
	thumbURL string
}

func (t *Task) buildPageURL(page int) string {
	g := strings.TrimSpace(t.genre)
	if g == "" {
		return ""
	}
	return fmt.Sprintf("https://hanime1.me/search?genre=%s&page=%d", url.QueryEscape(g), page)
}

func (t *Task) runScraper(url string) (string, error) {
	return downloader.ScraperNative(url, t.cookie)
}

type titleFallback int

const (
	fallbackNone      titleFallback = 0
	fallbackImgAlt    titleFallback = 1
	fallbackTitleAttr titleFallback = 2
)

type itemParseConfig struct {
	linkSel         string
	titleSel        string
	thumbFromParent bool
	titleFallback   titleFallback
}

// makeItem creates a hanimeItem from a matched DOM selection using the given config.
func makeItem(s *goquery.Selection, cfg itemParseConfig) (hanimeItem, bool) {
	href := strings.TrimSpace(s.AttrOr("href", ""))
	if href == "" {
		return hanimeItem{}, false
	}
	if strings.HasPrefix(href, "/") {
		href = baseURL + href
	}
	title := extractItemTitle(s, cfg)
	title = strings.ReplaceAll(title, "/", "／")
	title = strings.TrimRight(title, ".")
	vid := extractVideoIDFromURL(href)
	return hanimeItem{
		href:     href,
		title:    fmt.Sprintf(titleWithVidFmt, vid, title),
		thumbURL: extractItemThumb(s, cfg),
	}, true
}

// extractItemTitle extracts the title using the configured selector and fallback.
func extractItemTitle(s *goquery.Selection, cfg itemParseConfig) string {
	title := strings.TrimSpace(s.Find(cfg.titleSel).First().Text())
	if title != "" {
		return title
	}
	if cfg.titleFallback == fallbackImgAlt {
		return strings.TrimSpace(s.Find("img").First().AttrOr("alt", ""))
	}
	if cfg.titleFallback == fallbackTitleAttr {
		return strings.TrimSpace(s.AttrOr("title", ""))
	}
	return ""
}

// extractItemThumb extracts the thumbnail URL, optionally from the parent element.
func extractItemThumb(s *goquery.Selection, cfg itemParseConfig) string {
	if cfg.thumbFromParent {
		return strings.TrimSpace(s.Parent().Find("img").First().AttrOr("src", ""))
	}
	return strings.TrimSpace(s.Find("img").First().AttrOr("src", ""))
}

// collectItems extracts all hanime items from a document matching the given config.
func collectItems(doc *goquery.Document, cfg itemParseConfig) []hanimeItem {
	items := make([]hanimeItem, 0, 24)
	doc.Find(cfg.linkSel).Each(func(i int, s *goquery.Selection) {
		item, ok := makeItem(s, cfg)
		if ok {
			items = append(items, item)
		}
	})
	return items
}

func (t *Task) parseHomePage(html string) ([]hanimeItem, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Logger().Error("hanime parse home page failed", logutil.LogKeyError, err)
		return nil, err
	}

	selectors := []itemParseConfig{
		{linkSel: "a[href^='/videos']", titleSel: ".title, .card-title, h3, .name", titleFallback: fallbackImgAlt},
		{linkSel: ".search-result__item a", titleSel: ".title, .card-title, h3, .name", thumbFromParent: true},
		{linkSel: "a[href*='watch?v=']", titleSel: ".title, .home-rows-videos-title", titleFallback: fallbackTitleAttr},
	}

	for _, cfg := range selectors {
		items := collectItems(doc, cfg)
		if len(items) > 0 {
			return items, nil
		}
	}
	return nil, nil
}

func (t *Task) parseTotalPages(html string) int {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return 0
	}
	max := 0
	doc.Find(".pagination a, .pagination__item").Each(func(i int, s *goquery.Selection) {
		txt := strings.TrimSpace(s.Text())
		if n, err := strconv.Atoi(txt); err == nil {
			if n > max {
				max = n
			}
		}
	})
	if max <= 0 {
		max = 1
	}
	return max
}

func (t *Task) createObjectFromItem(v hanimeItem) *model.DownloadObject {
	videoPath, _ := t.ResolvePath(v.title, "video")
	obj := &model.DownloadObject{
		TaskID:   t.ID(),
		URL:      v.href,
		SavePath: videoPath,
		Metadata: map[string]string{
			"title": v.title,
			"type":  "composite",
		},
		Extra: map[string]any{
			"thumb_url": v.thumbURL,
		},
		Status: model.StatusPending,
	}
	t.CheckAndRestoreStatus(obj)
	return obj
}

type hanimeDetail struct {
	title    string
	videoURL string
	imageURL string
	artist   string
	details  string
	date     string
	tags     []string
	playlist []hanimeItem
}

func (t *Task) parseVideoPage(pageURL string) (*hanimeDetail, error) {
	html, err := t.runScraper(pageURL)
	if err != nil {
		return nil, err
	}
	return parseHanimeVideoPageHTML(pageURL, html)
}

func extractHanimeTitle(doc *goquery.Document, pageURL string) string {
	title := strings.TrimSpace(doc.Find("#shareBtn-title").First().Text())
	if title == "" {
		title = strings.TrimSpace(doc.Find("meta[property='og:title']").AttrOr("content", ""))
	}
	if title == "" {
		title = strings.TrimSpace(doc.Find("h1, .title, .video-title").First().Text())
	}
	vid := extractVideoIDFromURL(pageURL)
	title = fmt.Sprintf(titleWithVidFmt, vid, title)
	title = strings.ReplaceAll(title, "/", "／")
	title = strings.TrimRight(title, ".")
	return title
}

func extractHanimeArtist(doc *goquery.Document) string {
	return strings.TrimSpace(doc.Find("#video-artist-name").First().Text())
}

func extractHanimeDate(doc *goquery.Document) string {
	date := ""
	doc.Find(".video-details-wrapper .video-description-panel").First().Find("*").Each(func(i int, s *goquery.Selection) {
		if date != "" {
			return
		}
		v := strings.TrimSpace(s.Text())
		if strings.Contains(v, "觀看次數") && len(v) > 10 {
			date = strings.TrimSpace(v[len(v)-10:])
		}
	})
	return date
}

func extractHanimeDescription(doc *goquery.Document) string {
	detailLines := make([]string, 0, 4)
	doc.Find(".video-details-wrapper .video-description-panel").First().Find("*").Each(func(i int, s *goquery.Selection) {
		v := strings.TrimSpace(s.Text())
		if strings.Contains(v, "觀看次數") && len(v) > 10 {
			return
		}
		if v != "" {
			detailLines = append(detailLines, v)
		}
	})
	if len(detailLines) == 0 {
		desc := strings.TrimSpace(doc.Find(".video-details-wrapper .video-caption-text").First().Text())
		if desc != "" {
			detailLines = append(detailLines, desc)
		}
	}
	return strings.Join(detailLines, "\n")
}

func extractHanimeCoverImage(doc *goquery.Document) string {
	imageURL := strings.TrimSpace(doc.Find("meta[property='og:image']").AttrOr("content", ""))
	if imageURL == "" {
		imageURL = strings.TrimSpace(doc.Find("video").First().AttrOr("poster", ""))
	}
	return imageURL
}

func extractHanimeTags(doc *goquery.Document) []string {
	var tags []string
	doc.Find(".tags a, .video-tags a").Each(func(i int, s *goquery.Selection) {
		v := strings.TrimSpace(s.Text())
		if v != "" {
			tags = append(tags, v)
		}
	})
	if len(tags) > 0 {
		return tags
	}
	doc.Find(".video-tags-wrapper .single-video-tag a").Each(func(i int, s *goquery.Selection) {
		txt := strings.TrimSpace(s.Text())
		if txt == "" {
			return
		}
		if idx := strings.Index(txt, "("); idx > 0 {
			txt = strings.TrimSpace(txt[:idx])
		}
		txt = strings.TrimSpace(strings.TrimSuffix(txt, "\u00a0")) // 去掉NBSP
		if txt != "" {
			tags = append(tags, txt)
		}
	})
	return tags
}

func extractHanimePlaylist(doc *goquery.Document) []hanimeItem {
	var playlist []hanimeItem
	doc.Find("#video-playlist-wrapper .related-watch-wrap, #playlist-scroll .related-watch-wrap").Each(func(i int, s *goquery.Selection) {
		href := strings.TrimSpace(s.Find("a.overlay").AttrOr("href", ""))
		if href == "" {
			href = strings.TrimSpace(s.Find("a").AttrOr("href", ""))
		}
		if href == "" {
			return
		}
		if strings.HasPrefix(href, "/") {
			href = "https://hanime.tv" + href
		}
		title := strings.TrimSpace(s.Find(".card-mobile-title").First().Text())
		if title == "" {
			title = strings.TrimSpace(s.Find("img[alt]").First().AttrOr("alt", ""))
		}
		title = strings.ReplaceAll(title, "/", "／")
		title = strings.TrimRight(title, ".")
		thumb := ""
		images := s.Find("img")
		if images.Length() > 1 {
			thumb = strings.TrimSpace(images.Eq(1).AttrOr("src", ""))
		} else {
			thumb = strings.TrimSpace(images.First().AttrOr("src", ""))
		}
		playlist = append(playlist, hanimeItem{href: href, title: title, thumbURL: thumb})
	})
	return playlist
}

func extractHanimeVideoURL(doc *goquery.Document) (string, error) {
	vurl := strings.TrimSpace(doc.Find("video source").First().AttrOr("src", ""))
	if vurl != "" {
		return vurl, nil
	}
	scripts := doc.Find("script")
	reM3U8 := regexp.MustCompile(`https?://[^\s'"]+\.m3u8[^\s'"]*`)
	reMP4 := regexp.MustCompile(`https?://[^\s'"]+\.mp4[^\s'"]*`)
	scripts.Each(func(i int, s *goquery.Selection) {
		if vurl != "" {
			return
		}
		text := s.Text()
		if text == "" {
			return
		}
		if u := reM3U8.FindString(text); u != "" {
			vurl = u
			return
		}
		if u := reMP4.FindString(text); u != "" {
			vurl = u
			return
		}
		if strings.Contains(text, "hls_url") {
			idx := strings.Index(text, "hls_url")
			if idx >= 0 {
				rest := text[idx:]
				m := regexp.MustCompile(`["'](https?://[^"']+)["']`).FindStringSubmatch(rest)
				if len(m) >= 2 {
					vurl = m[1]
				}
			}
		}
	})
	if vurl == "" {
		return "", fmt.Errorf("video url not found")
	}
	return vurl, nil
}

func parseHanimeVideoPageHTML(pageURL, html string) (*hanimeDetail, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	info := &hanimeDetail{
		title:    extractHanimeTitle(doc, pageURL),
		artist:   extractHanimeArtist(doc),
		date:     extractHanimeDate(doc),
		details:  extractHanimeDescription(doc),
		imageURL: extractHanimeCoverImage(doc),
		tags:     extractHanimeTags(doc),
		playlist: extractHanimePlaylist(doc),
	}

	vurl, err := extractHanimeVideoURL(doc)
	if err != nil {
		return nil, err
	}
	info.videoURL = vurl
	return info, nil
}

func (t *Task) ResolveObject(_ context.Context, obj *model.DownloadObject) error {
	return t.resolveObject(obj, true)
}

func (t *Task) resolveObject(obj *model.DownloadObject, lock bool) error {
	info, err := t.parseVideoPage(obj.URL)
	if err != nil {
		return err
	}
	baseName := info.title
	baseName = strings.ReplaceAll(baseName, "/", "／")
	baseName = strings.TrimRight(baseName, ".")
	videoPath := filepath.Join(t.SaveDir(), baseName+".mp4")
	thumbPath := filepath.Join(t.SaveDir(), baseName+"_thumbnail.jpg")
	coverPath := filepath.Join(t.SaveDir(), baseName+"_cover.jpg")
	files := []map[string]string{}
	// 封面
	if info.imageURL != "" {
		files = append(files, map[string]string{
			"url":  info.imageURL,
			"path": coverPath,
			"type": "image",
		})
	}
	// 缩略图（来自搜索项的 thumb_url）
	if tu, ok := obj.Extra["thumb_url"].(string); ok && tu != "" {
		files = append(files, map[string]string{
			"url":  tu,
			"path": thumbPath,
			"type": "image",
		})
	}
	if info.videoURL != "" {
		files = append(files, map[string]string{
			"url":  info.videoURL,
			"path": videoPath,
			"type": "video",
		})
	}
	if lock {
		t.resolveApplyLocked(obj, info, files, videoPath)
	} else {
		t.resolveApply(obj, info, files, videoPath)
	}
	return nil
}

// resolveApply sets resolved metadata and extras onto the download object.
func (t *Task) resolveApply(obj *model.DownloadObject, info *hanimeDetail, files []map[string]string, videoPath string) {
	obj.Metadata[model.MetadataKeyTitle] = info.title
	obj.Metadata["date"] = info.date
	obj.SavePath = videoPath
	if _, ok := obj.Extra["files"]; !ok {
		obj.Extra["files"] = files
	}
	obj.Extra["tags"] = info.tags
	if info.artist != "" {
		obj.Extra["artist"] = info.artist
	}
	if info.details != "" {
		obj.Extra["details"] = info.details
	}
	if len(info.playlist) > 0 {
		pl := make([]map[string]string, 0, len(info.playlist))
		for _, it := range info.playlist {
			pl = append(pl, map[string]string{
				"url":   it.href,
				"title": it.title,
				"thumb": it.thumbURL,
			})
		}
		obj.Extra["playlist"] = pl
	}
}

// resolveApplyLocked wraps resolveApply with the manager lock.
func (t *Task) resolveApplyLocked(obj *model.DownloadObject, info *hanimeDetail, files []map[string]string, videoPath string) {
	t.WithLock(func() { t.resolveApply(obj, info, files, videoPath) })
}

func extractVideoIDFromURL(u string) string {
	// 期望形式 https://hanime1.me/watch?v=404480
	if u == "" {
		return ""
	}
	if strings.Contains(u, "?") {
		parts := strings.Split(u, "?")
		if len(parts) >= 2 {
			q := parts[1]
			for kv := range strings.SplitSeq(q, "&") {
				p := strings.SplitN(kv, "=", 2)
				if len(p) == 2 && (p[0] == "v" || p[0] == "video_id" || p[0] == "id") {
					return p[1]
				}
			}
		}
	}
	// 兜底：提取末尾数字序列
	re := regexp.MustCompile(`(\d{3,})`)
	if m := re.FindStringSubmatch(u); len(m) >= 2 {
		return m[1]
	}
	return ""
}

func hanimeItemURLs(items []hanimeItem) []string {
	urls := make([]string, 0, len(items))
	for _, item := range items {
		if item.href == "" {
			continue
		}
		urls = append(urls, item.href)
	}
	return urls
}
