// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package hanime

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/downloader"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/configutil"
	"github.com/cocomhub/download-manager/pkg/dlcore"
	"github.com/cocomhub/download-manager/task"

	"github.com/PuerkitoBio/goquery"
)

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
	genre    string
	cookie   string
	onceInit sync.Once
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
	t.SetPager(task.NewCommonPager(task.PageFuncs{
		BuildPageURL:    t.buildPageURL,
		RunScraper:      t.runScraper,
		ParseHomePage:   func(html string) (any, error) { return t.parseHomePage(html) },
		ParseTotalPages: t.parseTotalPages,
		ProcessItems: func(items any) ([]any, bool) {
			vs, _ := items.([]hanimeItem)
			existing := t.StorageExistenceMap(hanimeItemURLs(vs), true)
			var pageNew []*model.DownloadObject
			allKnown := true
			for _, v := range vs {
				if existing[v.href] {
					t.Logger().Info("hanime item already known", "url", v.href)
					continue
				}
				allKnown = false
				obj := t.createObjectFromItem(v)
				t.PersistTaskObject(obj)
				t.RememberRuntimeObject(obj, true)
				pageNew = append(pageNew, obj)
			}
			out := make([]any, len(pageNew))
			for i := range pageNew {
				out[i] = pageNew[i]
			}
			return out, allKnown
		},
	}))
	t.SetRefreshFunc(t.refreshLatest)
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
	if t.TryBeginInit() {
		go t.scrapeAllPages()
		return []*model.DownloadObject{}, nil
	}
	if !t.IsInitialized() {
		return []*model.DownloadObject{}, nil
	}

	t.onceInit.Do(func() {
		// Batch resolve objects that lack metadata (e.g. date).
		// Uses WithObjectsLock for safe access to the runtime objects list.
		t.WithObjectsLock(func(objs []*model.DownloadObject) {
			var toResolve []*model.DownloadObject
			for _, obj := range objs {
				if _, ok := obj.Metadata["date"]; !ok {
					toResolve = append(toResolve, obj)
				}
			}
			if len(toResolve) == 0 {
				return
			}
			sem := make(chan struct{}, 10)
			var wg sync.WaitGroup
			for _, obj := range toResolve {
				wg.Add(1)
				go func(o *model.DownloadObject) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					tmpObj := &model.DownloadObject{
						URL:      o.URL,
						Metadata: map[string]string{},
						Extra:    map[string]any{},
					}
					if err := t.resolveObject(tmpObj, false); err != nil {
						t.Logger().Error("[GG]hanime resolve object failed", "url", o.URL, "error", err)
						return
					}
					t.WithLock(func() {
						o.Metadata = tmpObj.Metadata
					})
					t.FlushObject(o)
				}(obj)
			}
			wg.Wait()
		})
	})

	objects := t.SnapshotRuntimeObjects(true)
	candidates := make([]*model.DownloadObject, 0)
	toResolve := make([]*model.DownloadObject, 0)
	activeCount := 0
	if t.Storage() != nil {
		if stored, err := t.Storage().Search(&core.StorageQuery{
			Filter: core.StorageFilter{
				TaskIDs:  []string{t.ID()},
				Statuses: []string{dlcore.StatusPending, dlcore.StatusFailed},
			},
			Sort:  []core.StorageSort{{Field: "date", Desc: true}, {Field: "url"}},
			Limit: 64,
		}); err == nil {
			objects = stored
			for _, obj := range stored {
				t.RememberRuntimeObject(obj, true)
			}
		}
	}
	for _, obj := range objects {
		if obj.Status == dlcore.StatusDownloading {
			activeCount++
		}
	}
	for _, obj := range objects {
		if t.IsMarkedFailed(obj.URL) {
			continue
		}
		if obj.Status != dlcore.StatusCompleted && obj.Status != dlcore.StatusCancelled {
			if _, hasFiles := obj.Extra["files"]; hasFiles {
				candidates = append(candidates, obj)
			} else {
				if len(candidates)+len(toResolve)+activeCount < t.Concurrency()*2+2 {
					toResolve = append(toResolve, obj)
				}
			}
		}
	}
	if len(toResolve) > 0 {
		for _, obj := range toResolve {
			if err := t.ResolveObject(obj); err == nil {
				candidates = append(candidates, obj)
			} else {
				t.Logger().Error("hanime resolve object failed", "url", obj.URL, "error", err)
				t.UpdateStatus(obj, dlcore.StatusFailed, err)
			}
		}
	}
	return candidates, nil
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

func (t *Task) parseHomePage(html string) ([]hanimeItem, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Logger().Error("hanime parse home page failed", "error", err)
		return nil, err
	}
	items := make([]hanimeItem, 0, 24)
	doc.Find("a[href^='/videos']").Each(func(i int, s *goquery.Selection) {
		href := strings.TrimSpace(s.AttrOr("href", ""))
		if href == "" {
			return
		}
		if strings.HasPrefix(href, "/") {
			href = "https://hanime1.me" + href
		}
		title := strings.TrimSpace(s.Find(".title, .card-title, h3, .name").First().Text())
		if title == "" {
			img := s.Find("img").First()
			alt := strings.TrimSpace(img.AttrOr("alt", ""))
			title = alt
		}
		title = strings.ReplaceAll(title, "/", "／")
		title = strings.TrimRight(title, ".")
		vid := extractVideoIDFromURL(href)
		title = fmt.Sprintf("[%s] %s", vid, title)
		thumb := strings.TrimSpace(s.Find("img").First().AttrOr("src", ""))
		items = append(items, hanimeItem{href: href, title: title, thumbURL: thumb})
	})
	if len(items) == 0 {
		doc.Find(".search-result__item a").Each(func(i int, s *goquery.Selection) {
			h := strings.TrimSpace(s.AttrOr("href", ""))
			if h == "" {
				return
			}
			if strings.HasPrefix(h, "/") {
				h = "https://hanime1.me" + h
			}
			title := strings.TrimSpace(s.Find(".title, .card-title, h3, .name").First().Text())
			title = strings.ReplaceAll(title, "/", "／")
			title = strings.TrimRight(title, ".")
			vid := extractVideoIDFromURL(h)
			title = fmt.Sprintf("[%s] %s", vid, title)
			thumb := strings.TrimSpace(s.Parent().Find("img").First().AttrOr("src", ""))
			items = append(items, hanimeItem{href: h, title: title, thumbURL: thumb})
		})
	}
	if len(items) == 0 {
		doc.Find("a[href*='watch?v=']").Each(func(i int, s *goquery.Selection) {
			h := strings.TrimSpace(s.AttrOr("href", ""))
			if h == "" {
				return
			}
			if strings.HasPrefix(h, "/") {
				h = "https://hanime1.me" + h
			}
			title := strings.TrimSpace(s.Find(".title, .home-rows-videos-title").First().Text())
			if title == "" {
				title = strings.TrimSpace(s.AttrOr("title", ""))
			}
			title = strings.ReplaceAll(title, "/", "／")
			title = strings.TrimRight(title, ".")
			vid := extractVideoIDFromURL(h)
			title = fmt.Sprintf("[%s] %s", vid, title)
			thumb := strings.TrimSpace(s.Find("img").First().AttrOr("src", ""))
			items = append(items, hanimeItem{href: h, title: title, thumbURL: thumb})
		})
	}
	return items, nil
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
		Status: dlcore.StatusPending,
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

func parseHanimeVideoPageHTML(pageURL, html string) (*hanimeDetail, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}
	info := &hanimeDetail{}
	// 标题
	title := strings.TrimSpace(doc.Find("#shareBtn-title").First().Text())
	if title == "" {
		title = strings.TrimSpace(doc.Find("meta[property='og:title']").AttrOr("content", ""))
	}
	if title == "" {
		title = strings.TrimSpace(doc.Find("h1, .title, .video-title").First().Text())
	}
	vid := extractVideoIDFromURL(pageURL)
	info.title = fmt.Sprintf("[%s] %s", vid, title)
	info.title = strings.ReplaceAll(info.title, "/", "／")
	info.title = strings.TrimRight(info.title, ".")
	// 作者/厂牌
	info.artist = strings.TrimSpace(doc.Find("#video-artist-name").First().Text())
	// 详情描述（视频详情面板中的文本）
	detailLines := make([]string, 0, 4)
	doc.Find(".video-details-wrapper .video-description-panel").First().Find("*").Each(func(i int, s *goquery.Selection) {
		v := strings.TrimSpace(s.Text())
		if strings.Contains(v, "觀看次數") && len(v) > 10 {
			info.date = strings.TrimSpace(v[len(v)-10:])
			return
		}
		if v != "" {
			detailLines = append(detailLines, v)
		}
	})
	if len(detailLines) == 0 {
		// 回退：直接读取描述文本块
		desc := strings.TrimSpace(doc.Find(".video-details-wrapper .video-caption-text").First().Text())
		if desc != "" {
			detailLines = append(detailLines, desc)
		}
	}
	info.details = strings.Join(detailLines, "\n")
	// 封面图
	info.imageURL = strings.TrimSpace(doc.Find("meta[property='og:image']").AttrOr("content", ""))
	if info.imageURL == "" {
		poster := strings.TrimSpace(doc.Find("video").First().AttrOr("poster", ""))
		info.imageURL = poster
	}
	// 标签
	doc.Find(".tags a, .video-tags a").Each(func(i int, s *goquery.Selection) {
		v := strings.TrimSpace(s.Text())
		if v != "" {
			info.tags = append(info.tags, v)
		}
	})
	// 若新站点结构（video-tags-wrapper）
	if len(info.tags) == 0 {
		doc.Find(".video-tags-wrapper .single-video-tag a").Each(func(i int, s *goquery.Selection) {
			txt := strings.TrimSpace(s.Text())
			if txt == "" {
				return
			}
			if idx := strings.Index(txt, "("); idx > 0 {
				txt = strings.TrimSpace(txt[:idx])
			}
			txt = strings.TrimSpace(strings.TrimSuffix(txt, " ")) // 去掉NBSP
			if txt != "" {
				info.tags = append(info.tags, txt)
			}
		})
	}
	// 播放列表（相关视频）
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
		// 第二张图通常是缩略图
		images := s.Find("img")
		if images.Length() > 1 {
			thumb = strings.TrimSpace(images.Eq(1).AttrOr("src", ""))
		} else {
			thumb = strings.TrimSpace(images.First().AttrOr("src", ""))
		}
		info.playlist = append(info.playlist, hanimeItem{href: href, title: title, thumbURL: thumb})
	})
	scripts := doc.Find("script")
	reM3U8 := regexp.MustCompile(`https?://[^\s'"]+\.m3u8[^\s'"]*`)
	reMP4 := regexp.MustCompile(`https?://[^\s'"]+\.mp4[^\s'"]*`)
	vurl := strings.TrimSpace(doc.Find("video source").First().AttrOr("src", ""))
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
					return
				}
			}
		}
	})
	if vurl == "" && info.date == "" {
		return nil, fmt.Errorf("video url not found")
	}
	info.videoURL = vurl
	return info, nil
}

func (t *Task) ResolveObject(obj *model.DownloadObject) error {
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
		typ := "video"
		if strings.Contains(info.videoURL, ".m3u8") {
			typ = "video"
		}
		files = append(files, map[string]string{
			"url":  info.videoURL,
			"path": videoPath,
			"type": typ,
		})
	}
	applyResolve := func() {
		obj.Metadata["title"] = info.title
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
	if lock {
		t.WithLock(applyResolve)
	} else {
		applyResolve()
	}
	return nil
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

func (t *Task) scrapeAllPages() {
	defer t.MarkInitialized()
parsePage1:
	page1 := t.buildPageURL(1)
	html, err := t.runScraper(page1)
	if err != nil {
		t.Logger().Error("hanime scrape page 1 failed", "url", page1, "error", err)
		goto parsePage1
	}
	os.WriteFile(t.genre+"_debug_hanime.html", []byte(html), 0644)
	total := t.parseTotalPages(html)
	if total == 0 {
		goto parsePage1
	}
	t.Logger().Info("hanime total pages", "url", page1, "total", total)
	items1, err := t.parseHomePage(html)
	if err == nil {
		existing := t.StorageExistenceMap(hanimeItemURLs(items1), true)
		for _, it := range items1 {
			if existing[it.href] {
				t.Logger().Info("hanime item already known", "url", it.href)
				continue
			}
			obj := t.createObjectFromItem(it)
			t.PersistTaskObject(obj)
			t.RememberRuntimeObject(obj, true)
		}
	}
	for i := 2; i <= total; i++ {
		u := t.buildPageURL(i)
		h, err := t.runScraper(u)
		if err != nil {
			t.Logger().Error("hanime scrape page failed", "url", u, "page", i, "error", err)
			i--
			continue
		}
		items, err := t.parseHomePage(h)
		if err != nil {
			t.Logger().Error("hanime parse home page failed", "url", u, "page", i, "error", err)
			i--
			continue
		}
		existing := t.StorageExistenceMap(hanimeItemURLs(items), true)
		for _, it := range items {
			if existing[it.href] {
				t.Logger().Info("hanime item already known", "url", it.href)
				continue
			}
			obj := t.createObjectFromItem(it)
			t.PersistTaskObject(obj)
			t.RememberRuntimeObject(obj, true)
		}
		t.Logger().Info("hanime parsed page", "url", u, "page", i, "items", len(items), "objects", t.RuntimeObjectCount())
	}
	t.Logger().Info("hanime scrape all pages done", "objects", t.RuntimeObjectCount())
}

func (t *Task) refreshLatest() {
	if !t.IsInitialized() {
		return
	}
	newAny, err := t.RefreshPager()
	if err != nil {
		t.Logger().Error("hanime refresh failed", "error", err)
		return
	}
	if len(newAny) == 0 {
		return
	}
	for i := range newAny {
		t.RememberRuntimeObject(newAny[i].(*model.DownloadObject), true)
	}
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
