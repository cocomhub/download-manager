// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package booksite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/task"
)

// booksiteAdapter bridges the booksite Task's site-specific methods
// to the task.SiteAdapter interface consumed by PagingScanner.
//
// 适配站点：xkcd（https://xkcd.com）——公开、无认证、JSON API。
// 分页模式：按漫画 ID 递增（page N → comic N），与 vikacg 相同使用
// 「BuildPageURL 编码 page → RunScraper 解码发请求」的合成 URL 模式。
type booksiteAdapter struct {
	t *Task
}

// Ensure adapter implements task.SiteAdapter.
var _ task.SiteAdapter = (*booksiteAdapter)(nil)

// BuildPageURL 构造第 page 页的 URL：编码为合成 URL，RunScraper 解码后请求 xkcd API。
func (a *booksiteAdapter) BuildPageURL(page int) string {
	return fmt.Sprintf("booksite://internal/page/%d", page)
}

// comicInfo xkcd API 响应结构。
type comicInfo struct {
	Num        int    `json:"num"`
	Title      string `json:"title"`
	Img        string `json:"img"`
	Alt        string `json:"alt"`
	Transcript string `json:"transcript"`
}

// RunScraper 从合成 URL 解码 page，请求 xkcd 单条 JSON API。
func (a *booksiteAdapter) RunScraper(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid booksite page URL: %s", rawURL)
	}
	page, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return "", fmt.Errorf("invalid booksite page number in URL: %s", rawURL)
	}

	apiURL := fmt.Sprintf("https://xkcd.com/%d/info.0.json", page)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "download-manager-booksite/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		// 超出最大 ID：返回空页（PagingScanner 空页熔断停止）。
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("xkcd API returned %d", resp.StatusCode)
	}
	var comic comicInfo
	if err := json.NewDecoder(resp.Body).Decode(&comic); err != nil {
		return "", err
	}
	data, err := json.Marshal([]comicInfo{comic})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ParseTotalPages xkcd 无列表页，未知返回 -1（PagingScanner 用空页熔断停止）。
func (a *booksiteAdapter) ParseTotalPages(html string) int {
	return -1
}

// ParsePage 从页面内容提取条目（漫画列表）。
func (a *booksiteAdapter) ParsePage(html string) (any, error) {
	if html == "" {
		return []comicInfo{}, nil
	}
	var comics []comicInfo
	if err := json.Unmarshal([]byte(html), &comics); err != nil {
		return nil, err
	}
	return comics, nil
}

// ItemsToURLs 从条目提取去重所需 URL。
func (a *booksiteAdapter) ItemsToURLs(items any) []string {
	comics := items.([]comicInfo)
	urls := make([]string, len(comics))
	for i, c := range comics {
		urls[i] = fmt.Sprintf("https://xkcd.com/%d/", c.Num)
	}
	return urls
}

// BuildObject 为第 index 个条目构建 DownloadObject。
// 缓存优先：先查 BaseTask.GetCachedObject，命中则复用。
func (a *booksiteAdapter) BuildObject(items any, index int) (*model.DownloadObject, error) {
	comics := items.([]comicInfo)
	if index < 0 || index >= len(comics) {
		return nil, fmt.Errorf("booksite: index %d out of range", index)
	}
	c := comics[index]
	u := fmt.Sprintf("https://xkcd.com/%d/", c.Num)

	if cached := a.t.GetCachedObject(u); cached != nil {
		cached.TaskID = a.t.ID()
		return cached, nil
	}

	obj := &model.DownloadObject{
		URL:    u,
		TaskID: a.t.ID(),
		Metadata: map[string]string{
			"title": c.Title,
		},
		Extra: map[string]any{
			"files": []any{
				map[string]string{"type": "image", "url": c.Img, "path": c.Img},
			},
			"page_url": u,
		},
	}
	obj.SetStatus(model.StatusPending)
	return obj, nil
}
