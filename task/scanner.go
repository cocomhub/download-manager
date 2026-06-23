// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"
	"log/slog"

	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/scrape"
)

// PagingScanner wraps scrape.Driver + SiteAdapter + BaseTask into a unified
// scrape 鈫?build 鈫?persist pipeline, replacing the per-task pagination boilerplate.
//
// Lifecycle:
//
//	PagingScanner.Run(ctx)
//	  鈫?driver.Scrape() with PageHooks from adapter
//	    鈫?per page: adapter.ParsePage 鈫?adapter.ItemsToURLs 鈫?ProcessNewURLs
//	      鈫?per new URL: adapter.BuildObject 鈫?CheckAndRestoreStatus 鈫?PersistTaskObject
//	  鈫?RememberRuntimeObject for each built object
type PagingScanner struct {
	driver  *scrape.Driver
	adapter SiteAdapter
	base    *BaseTask
	logger  *slog.Logger
}

// NewPagingScanner creates a PagingScanner bound to the given BaseTask and SiteAdapter.
// The scrape driver is injected later via SetDriver (when BaseTask.Scrape is called).
func NewPagingScanner(base *BaseTask, adapter SiteAdapter) *PagingScanner {
	return &PagingScanner{
		base:    base,
		adapter: adapter,
		logger:  base.Logger().With("component", "PagingScanner"),
	}
}

// SetDriver sets the scrape driver, typically called by BaseTask.Scrape.
func (s *PagingScanner) SetDriver(driver *scrape.Driver) {
	s.driver = driver
}

// Run executes a full or incremental scrape cycle, building and persisting
// DownloadObject instances for newly discovered URLs.
func (s *PagingScanner) Run(ctx context.Context) error {
	if s.driver == nil {
		s.logger.Debug("PagingScanner: no driver, skipping")
		return nil
	}
	hooks := s.buildHooks()
	opts := scrape.Options{
		MaxRetries:     3,
		MaxEmptyPages:  3,
		MaxTailRefresh: 5,
	}
	result := s.driver.Scrape(ctx, s.base.ID(), hooks, opts)
	if !result.AllSucceeded && result.LastFailedPage > 0 {
		s.logger.Warn("Scrape incomplete",
			"failed_page", result.LastFailedPage,
			"total_pages", result.DetectedPages)
	}
	for _, item := range result.Items {
		if obj, ok := item.(*model.DownloadObject); ok {
			s.base.RememberRuntimeObject(obj, true)
		}
	}
	return nil
}

// buildHooks constructs scrape.PageHooks from the adapter.
func (s *PagingScanner) buildHooks() scrape.PageHooks {
	return scrape.PageHooks{
		BuildPageURL:    s.adapter.BuildPageURL,
		RunScraper:      s.adapter.RunScraper,
		ParseTotalPages: s.adapter.ParseTotalPages,
		ParsePage:       s.adapter.ParsePage,
		ProcessItems:    s.processItems,
	}
}

// processItems implements scrape.PageHooks.ProcessItems.
// It deduplicates URLs via ProcessNewURLs, builds objects for unknown URLs,
// restores status via CheckAndRestoreStatus, and persists via PersistTaskObject.
func (s *PagingScanner) processItems(items any) ([]any, bool) {
	urls := s.adapter.ItemsToURLs(items)
	unknownURLs, allKnown := s.base.ProcessNewURLs(urls)
	if len(unknownURLs) == 0 {
		return nil, allKnown
	}

	unknownSet := make(map[string]bool, len(unknownURLs))
	for _, u := range unknownURLs {
		unknownSet[u] = true
	}

	var newObjects []any
	for i, u := range urls {
		if u == "" || !unknownSet[u] {
			continue
		}
		obj, err := s.adapter.BuildObject(items, i)
		if err != nil {
			s.logger.Warn("BuildObject failed", "url", u, "error", err)
			continue
		}
		if obj == nil {
			continue
		}
		s.base.CheckAndRestoreStatus(obj)
		s.base.PersistTaskObject(obj)
		newObjects = append(newObjects, obj)
	}
	return newObjects, allKnown
}
