// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package scrape

import (
	"context"
	"log/slog"
	"time"

	"github.com/cocomhub/download-manager/pkg/logutil"
)

// DefaultPager implements Pager with retry, dynamic tail refresh, maxEmptyPages break,
// and firstFailedPage tracking.
type DefaultPager struct{}

func NewDefaultPager() *DefaultPager {
	return &DefaultPager{}
}

func (p *DefaultPager) Run(ctx context.Context, hooks PageHooks, opts Options) Result {
	var all []any
	page := opts.StartPage
	if page <= 0 {
		page = 1
	}
	detectedPages := -1
	firstFailedPage := 0
	emptyPages := 0
	maxRetries := opts.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	maxEmpty := opts.MaxEmptyPages
	if maxEmpty <= 0 {
		maxEmpty = 3
	}
	maxRefresh := opts.MaxTailRefresh
	if maxRefresh <= 0 {
		maxRefresh = 5
	}
	refreshCount := 0

	var pageNew []any
	var allKnown bool
	var items any
	for {
		if err := ctx.Err(); err != nil {
			slog.Info("Pager: context canceled", "page", page, logutil.LogKeyError, err)
			return Result{Items: all, AllSucceeded: false, LastFailedPage: firstFailedPage, DetectedPages: detectedPages}
		}

		url := hooks.BuildPageURL(page)

		// Scrape page with retries
		var html string
		var err error
		for attempt := range maxRetries {
			html, err = hooks.RunScraper(url)
			if err == nil {
				break
			}
			if attempt < maxRetries-1 {
				slog.Warn("Pager: retry after scrape error", "page", page, logutil.LogKeyURL, url, "attempt", attempt+1, logutil.LogKeyError, err)
				if !sleepCtx(ctx, time.Duration(1<<attempt)*time.Second) {
					return Result{Items: all, AllSucceeded: false, LastFailedPage: firstFailedPage, DetectedPages: detectedPages}
				}
			}
		}
		if err != nil {
			slog.Error("Pager: scrape failed after max retries", "page", page, logutil.LogKeyURL, url, logutil.LogKeyError, err)
			if firstFailedPage == 0 {
				firstFailedPage = page
			}
			// Continue to next page (don't abort) to collect progress
			goto nextPage
		}

		// Detect total pages on first page
		if detectedPages == -1 {
			detectedPages = hooks.ParseTotalPages(html)
			if detectedPages <= 0 {
				detectedPages = 1
			}
		}

		// Parse page items
		for attempt := range maxRetries {
			items, err = hooks.ParsePage(html)
			if err == nil {
				break
			}
			if attempt < maxRetries-1 {
				slog.Warn("Pager: retry after parse error", "page", page, logutil.LogKeyURL, url, "attempt", attempt+1, logutil.LogKeyError, err)
				reHTML, reErr := hooks.RunScraper(url)
				if reErr != nil {
					slog.Error("Pager: re-scrape failed after parse error", "page", page, logutil.LogKeyURL, url, logutil.LogKeyError, reErr)
					err = reErr
					break
				}
				html = reHTML
			}
		}
		if err != nil {
			slog.Error("Pager: parse failed after max retries", "page", page, logutil.LogKeyURL, url, logutil.LogKeyError, err)
			if firstFailedPage == 0 {
				firstFailedPage = page
			}
			goto nextPage
		}

		if items == nil {
			break
		}

		pageNew, allKnown = hooks.ProcessItems(items)
		if len(pageNew) > 0 {
			all = append(all, pageNew...)
			emptyPages = 0
		} else {
			emptyPages++
		}

		if allKnown {
			// If there were failures before this point, still mark as incomplete
			if firstFailedPage > 0 {
				return Result{Items: all, AllSucceeded: false, LastFailedPage: firstFailedPage, DetectedPages: detectedPages}
			}
			break
		}

		// Incremental mode: max empty pages guard
		if opts.Mode == ModeIncremental && emptyPages >= maxEmpty {
			if firstFailedPage > 0 {
				return Result{Items: all, AllSucceeded: false, LastFailedPage: firstFailedPage, DetectedPages: detectedPages}
			}
			break
		}

	nextPage:
		page++
		if detectedPages > 0 && page > detectedPages {
			// Full mode: dynamic tail refresh
			if opts.Mode == ModeFull && refreshCount < maxRefresh {
				newDetected := hooks.ParseTotalPages(hooks.BuildPageURL(1))
				if newDetected > detectedPages {
					slog.Info("Pager: detected page growth, extending scan", "old", detectedPages, "new", newDetected)
					detectedPages = newDetected
					refreshCount++
					continue // don't advance page since we're past end
				}
			}
			break
		}
	}

	allSucceeded := firstFailedPage == 0
	return Result{Items: all, AllSucceeded: allSucceeded, LastFailedPage: firstFailedPage, DetectedPages: detectedPages}
}

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
