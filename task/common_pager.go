// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"
	"log/slog"
	"time"
)

type PageFuncs struct {
	BuildPageURL    func(page int) string
	RunScraper      func(url string) (string, error)
	ParseHomePage   func(html string) (any, error)
	ParseTotalPages func(html string) int
	ProcessItems    func(items any) (newItems []any, allKnown bool)
}

type CommonPager struct {
	funcs PageFuncs
}

func NewCommonPager(funcs PageFuncs) *CommonPager {
	return &CommonPager{funcs: funcs}
}

// RefreshLatest iterates pages until allKnown or pages exhausted.
func (p *CommonPager) RefreshLatest() ([]any, error) {
	page := 1
	maxPages := -1
	var newObjects []any
	for {
		url := p.funcs.BuildPageURL(page)
		html, err := p.funcs.RunScraper(url)
		if err != nil {
			return newObjects, err
		}
		if maxPages == -1 {
			maxPages = p.funcs.ParseTotalPages(html)
			if maxPages <= 0 {
				maxPages = 1
			}
		}
		items, err := p.funcs.ParseHomePage(html)
		if err != nil {
			return newObjects, err
		}
		if items == nil {
			break
		}
		pageNew, allKnown := p.funcs.ProcessItems(items)
		if len(pageNew) > 0 {
			newObjects = append(newObjects, pageNew...)
		}
		if allKnown {
			break
		}
		page++
		if page > maxPages {
			break
		}
	}
	return newObjects, nil
}

// ScrapeAllPages iterates all pages from 1 to maxPages (or total detected),
// with limited retries per page. Stops early when allKnown or ctx is canceled.
// Returns all items discovered before cancellation/exhaustion.
func (p *CommonPager) ScrapeAllPages(ctx context.Context, maxPages int) []any {
	var all []any
	page := 1
	detectedPages := -1
	const maxRetries = 3
	for {
		if err := ctx.Err(); err != nil {
			slog.Info("ScrapeAllPages: context canceled", "page", page, "error", err)
			return all
		}

		url := p.funcs.BuildPageURL(page)

		var html string
		var err error
		for attempt := range maxRetries {
			html, err = p.funcs.RunScraper(url)
			if err == nil {
				break
			}
			if attempt < maxRetries-1 {
				slog.Warn("ScrapeAllPages: retry after scrape error", "page", page, "url", url, "attempt", attempt+1, "error", err)
				if !sleepCtx(ctx, time.Duration(1<<attempt)*time.Second) {
					return all
				}
			}
		}
		if err != nil {
			slog.Error("ScrapeAllPages: failed after max retries (scrape)", "page", page, "url", url, "error", err)
			break
		}

		if detectedPages == -1 {
			detectedPages = p.funcs.ParseTotalPages(html)
			if detectedPages <= 0 {
				detectedPages = 1
			}
			if maxPages > 0 && detectedPages > maxPages {
				detectedPages = maxPages
			}
		}

		var items any
		for attempt := range maxRetries {
			items, err = p.funcs.ParseHomePage(html)
			if err == nil {
				break
			}
			if attempt < maxRetries-1 {
				slog.Warn("ScrapeAllPages: retry after parse error", "page", page, "url", url, "attempt", attempt+1, "error", err)
				// Re-scrape the page once on parse failure
				reHTML, reErr := p.funcs.RunScraper(url)
				if reErr != nil {
					slog.Error("ScrapeAllPages: re-scrape failed after parse error", "page", page, "url", url, "error", reErr)
					err = reErr
					break
				}
				html = reHTML
			}
		}
		if err != nil {
			slog.Error("ScrapeAllPages: failed after max retries (parse)", "page", page, "url", url, "error", err)
			break
		}

		if items == nil {
			break
		}

		pageNew, allKnown := p.funcs.ProcessItems(items)
		if len(pageNew) > 0 {
			all = append(all, pageNew...)
		}
		if allKnown {
			break
		}

		page++
		if page > detectedPages {
			break
		}
	}
	return all
}

// sleepCtx sleeps for d or until ctx is canceled. Returns false if canceled.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
