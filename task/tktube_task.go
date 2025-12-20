package task

import (
	"bytes"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"download-manager/config"
	"download-manager/core"
	"download-manager/model"

	"github.com/PuerkitoBio/goquery"
	"github.com/dop251/goja"
)

const (
	DefaultScraperPath = "/Users/libing/Documents/trae_projects/tktube-python/bin/scraper_get"
)

// TktubeTask implements core.Task for Tktube
type TktubeTask struct {
	id          string
	taskType    string // "tag", "model", "search"
	keyword     string
	pageStart   int
	pageEnd     int
	saveDir     string
	scraperPath string
	concurrency int

	objects     []*model.DownloadObject
	videoItems  []videoItem // Queue of videos to be processed
	store       core.Storage
	mu          sync.Mutex
	initialized bool
}

// Ensure TktubeTask implements core.Task
var _ core.Task = &TktubeTask{}

func NewTktubeTask(cfg config.TaskConfig, store core.Storage) (*TktubeTask, error) {
	extra := cfg.Extra
	if extra == nil {
		extra = make(map[string]interface{})
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
		// yaml might decode as float64?
		if v, ok := extra[key].(float64); ok {
			return int(v)
		}
		return def
	}

	subtype := getString("subtype", "tag")
	keyword := getString("keyword", "")
	pageStart := getInt("page_start", 1)
	pageEnd := getInt("page_end", 1)
	scraperPath := getString("scraper_path", DefaultScraperPath)
	concurrency := getInt("max_concurrent", 2)

	return &TktubeTask{
		id:          cfg.ID,
		taskType:    subtype,
		keyword:     keyword,
		pageStart:   pageStart,
		pageEnd:     pageEnd,
		saveDir:     cfg.SaveDir,
		scraperPath: scraperPath,
		concurrency: concurrency,
		objects:     make([]*model.DownloadObject, 0),
		videoItems:  make([]videoItem, 0),
		store:       store,
	}, nil
}

func (t *TktubeTask) ID() string {
	return t.id
}

func (t *TktubeTask) Type() string {
	return "tktube_" + t.taskType
}

func (t *TktubeTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 1. Initialize (Scrape all pages) if not done
	if !t.initialized {
		// Reverse order: pageEnd -> pageStart
		for i := t.pageStart; i <= t.pageEnd; i++ {
			url := t.buildPageURL(i)
			slog.Info("Scraping page", "task_id", t.id, "page", i, "url", url)

			htmlContent, err := t.runScraper(url)
			if err != nil {
				slog.Error("Failed to scrape page", "task_id", t.id, "page", i, "error", err)
				continue
			}
			slog.Debug("Successfully scraped page", "task_id", t.id, "page", i, "size", len(htmlContent))

			videos, err := t.parseHomePage(htmlContent)
			if err != nil {
				slog.Error("Failed to parse page", "task_id", t.id, "page", i, "error", err)
				continue
			}

			// Append to videoItems
			t.videoItems = append(t.videoItems, videos...)
			slog.Info("Found videos on page", "task_id", t.id, "count", len(videos), "page", i)
		}
		for i := 0; i < len(t.videoItems)/2; i++ {
			t.videoItems[i], t.videoItems[len(t.videoItems)-1-i] = t.videoItems[len(t.videoItems)-1-i], t.videoItems[i]
		}
		t.initialized = true
		slog.Info("Initialization done", "task_id", t.id, "total_videos", len(t.videoItems))
	}

	// 2. Check current pending objects
	var pending []*model.DownloadObject
	for _, obj := range t.objects {
		if obj.Status != model.StatusCompleted {
			pending = append(pending, obj)
		}
		if len(pending) == t.GetConcurrency() {
			return pending, nil
		}
	}

	// 3. Replenish if low on pending objects
	// "return 3 each time" - implied buffer size
	for len(t.videoItems) > 0 && len(pending) < t.GetConcurrency() {
		// Pop video item
		v := t.videoItems[0]
		t.videoItems = t.videoItems[1:]

		// Deduplication
		if t.store != nil {
			if storedObj, err := t.store.Get(v.href); err == nil && storedObj != nil {
				t.objects = append(t.objects, storedObj)
				if storedObj.Status == model.StatusCompleted {
					slog.Debug("Already completed", "task_id", t.id, "title", v.title)
					continue
				}
			}
		}

		// Parse details (Blocking call)
		slog.Info("Parsing video details", "task_id", t.id, "title", v.title)
		videoInfo, err := t.parseVideoPage(v.href)
		if err != nil {
			slog.Error("Failed to parse video", "task_id", t.id, "title", v.title, "error", err)
			continue
		}

		// Create Single Composite Download Object
		// ID = Page URL (v.href)
		// Files to download = Video + Image

		// Determine paths relative to SaveDir or absolute?
		// Generally SaveDir + Keyword + Title.ext
		baseName := videoInfo.title
		// Sanitize filename
		baseName = strings.ReplaceAll(baseName, "/", "_")

		// We can store the list of files in Extra
		files := []map[string]string{
			{
				"url":  videoInfo.imageURL,
				"path": filepath.Join(t.saveDir, t.keyword, baseName+".jpg"),
				"type": "image",
			},
			{
				"url":  videoInfo.videoURL,
				"path": filepath.Join(t.saveDir, t.keyword, baseName+".mp4"),
				"type": "video",
			},
		}

		obj := &model.DownloadObject{
			TaskID:   t.id,
			URL:      v.href, // Page URL as ID
			SavePath: "",     // Not used for composite object, or use directory?
			Metadata: map[string]string{
				"title":    videoInfo.title,
				"page_url": v.href,
				"type":     "composite",
			},
			Extra: map[string]interface{}{
				"tags":  videoInfo.tags,
				"files": files,
			},
			Status: model.StatusPending,
		}

		// Check storage again to restore partial state if supported?
		// Currently we only support "Completed" or "Pending/Failed".
		// If we restart, we might re-download.
		// Refinement: If storage has this Page URL, restore its status.
		t.checkAndRestoreStatus(obj)

		t.objects = append(t.objects, obj)
		pending = append(pending, obj)
	}

	return pending, nil
}

func (t *TktubeTask) checkAndRestoreStatus(obj *model.DownloadObject) {
	if t.store != nil {
		if storedObj, err := t.store.Get(obj.URL); err == nil && storedObj != nil {
			obj.Status = storedObj.Status
			obj.Metadata = storedObj.Metadata
			obj.Extra = storedObj.Extra
		}
	}
}

func (t *TktubeTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	obj.Status = status
	if t.store != nil {
		return t.store.Update(obj)
	}
	return nil
}

func (t *TktubeTask) GetAllObjects() []*model.DownloadObject {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.objects
}

// GetConcurrency returns the configured concurrency limit for this task
func (t *TktubeTask) GetConcurrency() int {
	return t.concurrency
}

// --- Helpers ---

func (t *TktubeTask) buildPageURL(page int) string {
	ts := time.Now().UnixMilli()
	switch t.taskType {
	case "tag":
		return fmt.Sprintf("https://tktube.com/tags/%s/?mode=async&function=get_block&block_id=list_videos_common_videos_list&sort_by=post_date&from=%d&_=%d", t.keyword, page, ts)
	case "model":
		return fmt.Sprintf("https://tktube.com/models/%s/?mode=async&function=get_block&block_id=list_videos_common_videos_list&sort_by=post_date&from=%d&_=%d", t.keyword, page, ts)
	case "search":
		return fmt.Sprintf("https://tktube.com/zh/search/?q=%s&mode=async&function=get_block&block_id=list_videos_videos_list_search_result&category_ids=&sort_by=post_date&from_videos=%d&from_albums=%d&_=%d", t.keyword, page, page, ts)
	default:
		return ""
	}
}

func (t *TktubeTask) runScraper(url string) (string, error) {
	cmd := exec.Command(t.scraperPath, url)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("scraper failed: %v, stderr: %s", err, stderr.String())
	}
	return out.String(), nil
}

type videoItem struct {
	href  string
	title string
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

		var title string
		s.Find(".title").First().Each(func(i int, s *goquery.Selection) {
			title = strings.TrimSpace(s.Text())
		})
		items = append(items, videoItem{href: href, title: title})
	})
	return items, nil
}

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
	info.title = doc.Find("h1").Text()

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
		return nil, fmt.Errorf("flashvars script not found")
	}

	// Setup JS VM
	vm := goja.New()

	vm.Set("window", map[string]interface{}{
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
	resultArray, ok := resultExport.([]interface{})
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
