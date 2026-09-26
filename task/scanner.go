// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"
	"log/slog"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/configutil"
	"github.com/cocomhub/download-manager/pkg/logutil"
	"github.com/cocomhub/download-manager/pkg/scrape"
)

// PagingScanner wraps scrape.Driver + SiteAdapter + BaseTask into a unified
// scrape → build → persist pipeline, replacing the per-task pagination boilerplate.
//
// Lifecycle:
//
//	PagingScanner.Run(ctx)
//	  → driver.Scrape() with PageHooks from adapter
//	    → per page: adapter.ParsePage → adapter.ItemsToURLs → ProcessNewURLs
//	      → per new URL: adapter.BuildObject → CheckAndRestoreStatus → PersistTaskObject
//	  → RememberRuntimeObject for each built object
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
	// max_pages 页数硬上限（extra 配置，0=无限）：防止无 total-pages 的站点（如
	// ParseTotalPages=-1）首次扫描无限翻页卡死。
	maxPages := int(configutil.GetInt64(s.base.Extra, "max_pages", 0))
	opts := scrape.Options{
		MaxRetries:     3,
		MaxEmptyPages:  3,
		MaxTailRefresh: 5,
		MaxPages:       maxPages,
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
	// 抓取后立即补详情：对刚 built 的对象执行 ObjectVersioner 升级
	// （version=0 → LatestVersion），不依赖下次重启的 runVersionUpgrade。
	// 每个对象只补一次（setVersion 后不再补）；失败不阻塞（下次启动再补）。
	if ov, ok := s.adapter.(core.ObjectVersioner); ok {
		latest := int64(ov.LatestVersion())
		if latest > 0 {
			ov.BeginUpgrade()
			updated := 0
			for _, item := range result.Items {
				obj, ok := item.(*model.DownloadObject)
				if !ok || obj == nil {
					continue
				}
				cur := obj.GetVersion()
				if cur >= latest {
					continue
				}
				for v := cur + 1; v <= latest; v++ {
					if _, err := ov.UpgradeStep(obj, int(v)); err != nil {
						s.logger.Warn("Immediate upgrade failed", logutil.LogKeyURL, obj.URL, "to_version", v, logutil.LogKeyError, err)
						break
					}
				}
				obj.SetVersion(latest)
				if err := s.base.UpdateStatus(obj, obj.Status, nil); err != nil {
					s.logger.Warn("Immediate upgrade persist failed", logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
					continue
				}
				updated++
			}
			if updated > 0 {
				s.logger.Info("Scrape: immediate detail upgrade", "updated", updated, "total", len(result.Items))
			}
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
		start := time.Now()
		obj, err := s.adapter.BuildObject(items, i)
		if err != nil {
			s.logger.Warn("BuildObject failed", logutil.LogKeyURL, u, logutil.LogKeyError, err, "dur_ms", time.Since(start).Milliseconds())
			continue
		}
		if obj == nil {
			s.logger.Debug("BuildObject skipped", logutil.LogKeyURL, u, "dur_ms", time.Since(start).Milliseconds())
			continue
		}
		s.base.CheckAndRestoreStatus(obj)
		s.base.PersistTaskObject(obj)
		s.logger.Debug("BuildObject built", logutil.LogKeyURL, u, "task_type", obj.Metadata["task_type"], "dur_ms", time.Since(start).Milliseconds())
		newObjects = append(newObjects, obj)
	}
	return newObjects, allKnown
}
