// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tktube

import (
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/task"
)

// tktubeAdapter bridges the tktube Task's existing internal methods
// to the task.SiteAdapter interface.
type tktubeAdapter struct {
	t *Task
}

func (a *tktubeAdapter) BuildPageURL(page int) string {
	return a.t.buildPageURL(page)
}

func (a *tktubeAdapter) RunScraper(url string) (string, error) {
	return a.t.runScraper(url)
}

func (a *tktubeAdapter) ParseTotalPages(html string) int {
	return a.t.parseTotalPages(html)
}

func (a *tktubeAdapter) ParsePage(html string) (any, error) {
	return a.t.parseHomePage(html) // returns []videoItem
}

func (a *tktubeAdapter) ItemsToURLs(items any) []string {
	return videoItemURLs(items.([]videoItem))
}

func (a *tktubeAdapter) BuildObject(items any, index int) (*model.DownloadObject, error) {
	list := items.([]videoItem)
	return a.t.createObjectFromVideoItem(list[index]), nil
}

// Ensure adapter implements task.SiteAdapter.
var _ task.SiteAdapter = (*tktubeAdapter)(nil)
