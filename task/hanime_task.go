package task

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"download-manager/config"
	"download-manager/core"
	"download-manager/downloader"
	"download-manager/model"

	"github.com/PuerkitoBio/goquery"
)

type HanimeTask struct {
	id           string
	genre        string
	cookie       string
	saveDir      string
	concurrency  int
	refreshInt   int
	store        core.Storage
	shared       core.SharedRegistry
	objects      []*model.DownloadObject
	mu           sync.Mutex
	initialized  atomic.Int32
	hasUpdate    atomic.Bool
	knownURLs    map[string]bool
	pathStrategy core.PathStrategy
	refresher    *CommonRefresher
	pager        *CommonPager
}

var _ core.Task = &HanimeTask{}

func NewHanimeTask(cfg config.Task, store core.Storage) (*HanimeTask, error) {
	extra := cfg.Extra
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
	getBool := func(key string, def bool) bool {
		if extra == nil {
			return def
		}
		if v, ok := extra[key].(bool); ok {
			return v
		}
		return def
	}
	genre := getString("genre", "裏番")
	saveDir := cfg.SaveDir
	if getBool("save_dir_add_genre", false) && genre != "" {
		saveDir = filepath.Join(cfg.SaveDir, genre)
	}
	psMode := getString("path_strategy", "first_fixed")

	t := &HanimeTask{
		id:          cfg.ID,
		genre:       genre,
		cookie:      getString("cookie", ""),
		saveDir:     saveDir,
		concurrency: getInt("max_concurrent", 2),
		refreshInt:  getInt("refresh_interval", 3600),
		store:       store,
		objects:     make([]*model.DownloadObject, 0),
		knownURLs:   make(map[string]bool),
	}
	t.pathStrategy = NewPathStrategy(psMode, saveDir)
	t.pager = NewCommonPager(PageFuncs{
		BuildPageURL:    t.buildPageURL,
		RunScraper:      t.runScraper,
		ParseHomePage:   func(html string) (interface{}, error) { return t.parseHomePage(html) },
		ParseTotalPages: t.parseTotalPages,
		ProcessItems: func(items interface{}) ([]interface{}, bool) {
			vs, _ := items.([]hanimeItem)
			t.mu.Lock()
			defer t.mu.Unlock()
			var pageNew []*model.DownloadObject
			allKnown := true
			for _, v := range vs {
				if t.knownURLs[v.href] {
					slog.Info("hanime item already known", "task_id", t.id, "url", v.href)
					continue
				}
				allKnown = false
				obj := t.createObjectFromItem(v)
				t.knownURLs[v.href] = true
				pageNew = append(pageNew, obj)
			}
			out := make([]interface{}, len(pageNew))
			for i := range pageNew {
				out[i] = pageNew[i]
			}
			return out, allKnown
		},
	})
	t.refresher = NewCommonRefresher(t.refreshInt)
	t.refresher.Start(t.refreshLatest)
	return t, nil
}

func (t *HanimeTask) ID() string {
	return t.id
}

func (t *HanimeTask) Type() string {
	return "hanime_search"
}

func (t *HanimeTask) Close() error {
	if t.store != nil {
		if flusher, ok := t.store.(interface{ ForceFlush() error }); ok {
			if err := flusher.ForceFlush(); err != nil {
				slog.Error("hanime force flush store failed", "task_id", t.id, "error", err)
				return err
			}
		}
	}
	if t.refresher != nil {
		t.refresher.Stop()
	}
	return nil
}

func (t *HanimeTask) SetSharedRegistry(reg core.SharedRegistry) {
	t.shared = reg
}

func (t *HanimeTask) GetDownloadHeaders() map[string]string {
	return map[string]string{
		"Cookie": t.cookie,
	}
}

func (t *HanimeTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	if t.initialized.CompareAndSwap(0, -1) {
		go t.scrapeAllPages()
		return []*model.DownloadObject{}, nil
	}
	if t.initialized.Load() != 1 {
		return []*model.DownloadObject{}, nil
	}
	t.mu.Lock()
	candidates := make([]*model.DownloadObject, 0)
	toResolve := make([]*model.DownloadObject, 0)
	activeCount := 0
	for _, obj := range t.objects {
		if obj.Status == model.StatusDownloading {
			activeCount++
		}
	}
	for _, obj := range t.objects {
		if obj.Status != model.StatusCompleted && obj.Status != model.StatusCancelled {
			if _, hasFiles := obj.Extra["files"]; hasFiles {
				candidates = append(candidates, obj)
			} else {
				if len(candidates)+len(toResolve)+activeCount < t.concurrency*2+2 {
					toResolve = append(toResolve, obj)
				}
			}
		}
	}
	t.mu.Unlock()
	if len(toResolve) > 0 {
		for _, obj := range toResolve {
			if err := t.ResolveObject(obj); err == nil {
				candidates = append(candidates, obj)
			} else {
				t.UpdateStatus(obj, model.StatusFailed, err)
			}
		}
	}
	return candidates, nil
}

func (t *HanimeTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	obj.Status = status
	var e error
	if t.store != nil {
		e = t.store.Update(obj)
	}
	if t.shared != nil {
		_ = t.shared.Update(obj)
	}
	t.hasUpdate.Store(true)
	return e
}

func (t *HanimeTask) GetAllObjects() []*model.DownloadObject {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.objects
}

func (t *HanimeTask) getCachePath() string {
	return filepath.Join(config.GetWorkDir(), "cache", t.id+".json")
}

func (t *HanimeTask) SaveCache() error {
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
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	return os.WriteFile(path, data, 0644)
}

func (t *HanimeTask) LoadCache() error {
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
		slog.Error("hanime load cache failed", "task_id", t.id, "error", err)
		return err
	}
	var objects []*model.DownloadObject
	if err := json.Unmarshal(data, &objects); err != nil {
		slog.Error("hanime unmarshal cache failed", "task_id", t.id, "error", err)
		return err
	}
	t.objects = objects
	t.knownURLs = make(map[string]bool)
	for _, obj := range t.objects {
		slog.Info("hanime load cache item", "task_id", t.id, "url", obj.URL)
		t.knownURLs[obj.URL] = true
		if t.shared != nil {
			if so, err := t.shared.Get(obj.URL); err == nil && so != nil {
				*obj = *so
			}
		} else if t.store != nil {
			if so, err := t.store.Get(obj.URL); err == nil && so != nil {
				*obj = *so
			}
		}
		if obj.Status != model.StatusCompleted && obj.Status != model.StatusCancelled {
			if obj.Status == model.StatusDownloading {
				obj.Status = model.StatusPending
			}
			delete(obj.Extra, "files")
		}
	}
	return nil
}

type hanimeItem struct {
	href     string
	title    string
	thumbURL string
}

func (t *HanimeTask) buildPageURL(page int) string {
	g := strings.TrimSpace(t.genre)
	if g == "" {
		return ""
	}
	return fmt.Sprintf("https://hanime1.me/search?genre=%s&page=%d", urlEncodeGenre(g), page)
}

func (t *HanimeTask) runScraper(url string) (string, error) {
	return downloader.ScraperNative(url, t.cookie)
}

func (t *HanimeTask) parseHomePage(html string) ([]hanimeItem, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		slog.Error("hanime parse home page failed", "task_id", t.id, "error", err)
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

func (t *HanimeTask) parseTotalPages(html string) int {
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

func (t *HanimeTask) createObjectFromItem(v hanimeItem) *model.DownloadObject {
	videoPath, _ := t.pathStrategy.Resolve(t.saveDir, t.id, v.title, "video")
	obj := &model.DownloadObject{
		TaskID:   t.id,
		URL:      v.href,
		SavePath: videoPath,
		Metadata: map[string]string{
			"title":    v.title,
			"page_url": v.href,
			"type":     "composite",
		},
		Extra: map[string]interface{}{
			"thumb_url": v.thumbURL,
		},
		Status: model.StatusPending,
	}
	if t.shared != nil {
		if so, err := t.shared.Get(obj.URL); err == nil && so != nil {
			*obj = *so
			return obj
		}
	}
	if t.store != nil {
		if so, err := t.store.Get(obj.URL); err == nil && so != nil {
			*obj = *so
		}
	}
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

func (t *HanimeTask) parseVideoPage(pageURL string) (*hanimeDetail, error) {
	html, err := t.runScraper(pageURL)
	if err != nil {
		return nil, err
	}
	return parseVideoPageHTML(pageURL, html)
}

func parseVideoPageHTML(pageURL, html string) (*hanimeDetail, error) {
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
	if vurl == "" {
		return nil, fmt.Errorf("video url not found")
	}
	info.videoURL = vurl
	return info, nil
}

func (t *HanimeTask) ResolveObject(obj *model.DownloadObject) error {
	info, err := t.parseVideoPage(obj.URL)
	if err != nil {
		return err
	}
	baseName := info.title
	baseName = strings.ReplaceAll(baseName, "/", "／")
	baseName = strings.TrimRight(baseName, ".")
	videoPath := filepath.Join(t.saveDir, baseName+".mp4")
	thumbPath := filepath.Join(t.saveDir, baseName+"_thumbnail.jpg")
	coverPath := filepath.Join(t.saveDir, baseName+"_cover.jpg")
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
	t.mu.Lock()
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
	t.mu.Unlock()
	if t.store != nil {
		t.store.Update(obj)
	}
	if t.shared != nil {
		t.shared.Update(obj)
	}
	return nil
}

func urlEncodeGenre(g string) string {
	// Hanime 使用空格分隔的类型，如 "Motion Anime"
	// 使用 QueryEscape 进行编码
	return url.QueryEscape(g)
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
			for _, kv := range strings.Split(q, "&") {
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
func (t *HanimeTask) scrapeAllPages() {
	defer t.initialized.Store(1)
	t.mu.Lock()
parsePage1:
	page1 := t.buildPageURL(1)
	html, err := t.runScraper(page1)
	if err != nil {
		slog.Error("hanime scrape page 1 failed", "task_id", t.id, "url", page1, "error", err)
		goto parsePage1
	}
	os.WriteFile(t.genre+"_debug_hanime.html", []byte(html), 0644)
	total := t.parseTotalPages(html)
	if total == 0 {
		goto parsePage1
	}
	slog.Info("hanime total pages", "task_id", t.id, "url", page1, "total", total)
	items1, err := t.parseHomePage(html)
	if err == nil {
		for _, it := range items1 {
			if t.knownURLs[it.href] {
				slog.Info("hanime item already known", "task_id", t.id, "url", it.href)
				continue
			}
			obj := t.createObjectFromItem(it)
			t.objects = append(t.objects, obj)
			t.knownURLs[it.href] = true
		}
	}
	for i := 2; i <= total; i++ {
		u := t.buildPageURL(i)
		h, err := t.runScraper(u)
		if err != nil {
			slog.Error("hanime scrape page failed", "task_id", t.id, "url", u, "page", i, "error", err)
			i--
			continue
		}
		items, err := t.parseHomePage(h)
		if err != nil {
			slog.Error("hanime parse home page failed", "task_id", t.id, "url", u, "page", i, "error", err)
			i--
			continue
		}
		for _, it := range items {
			if t.knownURLs[it.href] {
				slog.Info("hanime item already known", "task_id", t.id, "url", it.href)
				continue
			}
			obj := t.createObjectFromItem(it)
			t.objects = append(t.objects, obj)
			t.knownURLs[it.href] = true
		}
		slog.Info("hanime parsed page", "task_id", t.id, "url", u, "page", i, "items", len(items), "objects", len(t.objects))
	}
	t.hasUpdate.Store(true)
	t.mu.Unlock()

	interval := 1 * time.Second
	for err := t.SaveCache(); err != nil; err = t.SaveCache() {
		slog.Error("hanime save cache failed", "task_id", t.id, "error", err)
		interval *= 2
		if interval > 1*time.Minute {
			interval = 1 * time.Minute
		}
		time.Sleep(interval)
	}
	slog.Info("hanime scrape all pages done", "task_id", t.id, "objects", len(t.objects))
}

func (t *HanimeTask) refreshLatest() {
	if t.initialized.Load() != 1 {
		return
	}
	newAny, err := t.pager.RefreshLatest()
	if err != nil {
		slog.Error("hanime refresh failed", "task_id", t.id, "error", err)
		return
	}
	if len(newAny) == 0 {
		return
	}
	t.mu.Lock()
	pageObjs := make([]*model.DownloadObject, len(newAny))
	for i := range newAny {
		pageObjs[i] = newAny[i].(*model.DownloadObject)
	}
	t.objects = append(pageObjs, t.objects...)
	t.mu.Unlock()
	if err := t.SaveCache(); err != nil {
		slog.Error("hanime save cache failed", "task_id", t.id, "error", err)
	}
}
