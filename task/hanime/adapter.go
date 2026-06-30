// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package hanime

import (
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/task"
)

// hanimeAdapter bridges the hanime Task's existing internal methods
// to the task.SiteAdapter interface.
type hanimeAdapter struct {
	t *Task
}

func (a *hanimeAdapter) BuildPageURL(page int) string {
	return a.t.buildPageURL(page)
}

func (a *hanimeAdapter) RunScraper(url string) (string, error) {
	return a.t.runScraper(url)
}

func (a *hanimeAdapter) ParseTotalPages(html string) int {
	return a.t.parseTotalPages(html)
}

func (a *hanimeAdapter) ParsePage(html string) (any, error) {
	return a.t.parseHomePage(html) // returns []hanimeItem
}

func (a *hanimeAdapter) ItemsToURLs(items any) []string {
	return hanimeItemURLs(items.([]hanimeItem))
}

func (a *hanimeAdapter) BuildObject(items any, index int) (*model.DownloadObject, error) {
	list := items.([]hanimeItem)
	return a.t.createObjectFromItem(list[index]), nil
}

// Ensure adapter implements task.SiteAdapter.
var _ task.SiteAdapter = (*hanimeAdapter)(nil)
