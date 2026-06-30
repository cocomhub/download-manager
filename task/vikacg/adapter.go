// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package vikacg

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/task"
)

// vikacgAdapter bridges the vikacg Task's existing internal methods
// to the task.SiteAdapter interface.
//
// Since vikacg uses a POST-based JSON API rather than traditional page URLs,
// BuildPageURL encodes the page number into the URL string, and RunScraper
// decodes it to make the actual API call.
type vikacgAdapter struct {
	t *Task
}

func (a *vikacgAdapter) BuildPageURL(page int) string {
	// Encode the page number as a synthetic URL for the adapter contract.
	// RunScraper will parse it back to make the actual API call.
	return fmt.Sprintf("vikacg://internal/page/%d", page)
}

func (a *vikacgAdapter) RunScraper(rawURL string) (string, error) {
	// Parse the synthetic URL to extract the page number
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid vikacg page URL: %s", rawURL)
	}
	page, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return "", fmt.Errorf("invalid vikacg page number in URL: %s", rawURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	posts, err := a.t.getPostsPage(ctx, page)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(posts)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *vikacgAdapter) ParseTotalPages(html string) int {
	// vikacg API does not return total page count.
	// The DefaultPager will use its circuit breaker (maxEmptyPages) to stop.
	return -1
}

func (a *vikacgAdapter) ParsePage(html string) (any, error) {
	var posts []vikPost
	if err := json.Unmarshal([]byte(html), &posts); err != nil {
		return nil, err
	}
	return posts, nil
}

func (a *vikacgAdapter) ItemsToURLs(items any) []string {
	return vikPostURLs(items.([]vikPost))
}

func (a *vikacgAdapter) BuildObject(items any, index int) (*model.DownloadObject, error) {
	list := items.([]vikPost)
	post := list[index]
	u := fmt.Sprintf("https://www.vikacg.com/p/%d", post.ID)

	// Check cache first
	if cached := a.t.GetCachedObject(u); cached != nil {
		cached.TaskID = a.t.ID()
		a.t.sanitizeCachedContentHTML(cached)
		return cached, nil
	}

	// Fetch and build from detail page
	return a.t.scrapeAndBuild(u)
}

// Ensure adapter implements task.SiteAdapter.
var _ task.SiteAdapter = (*vikacgAdapter)(nil)
