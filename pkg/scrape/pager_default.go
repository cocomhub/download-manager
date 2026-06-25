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

// pageState holds all mutable state for a pagination run.
type pageState struct {
	page            int
	detectedPages   int
	firstFailedPage int
	emptyPages      int
	refreshCount    int
	maxRetries      int
	maxEmpty        int
	maxRefresh      int
	mode            Mode
	canceled        bool
}

func newPageState(opts Options) *pageState {
	page := opts.StartPage
	if page <= 0 {
		page = 1
	}
	return &pageState{
		page:          page,
		detectedPages: -1,
		maxRetries:    defaultPositive(opts.MaxRetries, 3),
		maxEmpty:      defaultPositive(opts.MaxEmptyPages, 3),
		maxRefresh:    defaultPositive(opts.MaxTailRefresh, 5),
		mode:          opts.Mode,
	}
}

func defaultPositive(v, d int) int {
	if v <= 0 {
		return d
	}
	return v
}

// processAction signals what the main loop should do next.
type processAction int

const (
	actionContinue processAction = iota
	actionBreak
	actionReturn
)

func (st *pageState) result(all []any) Result {
	return Result{
		Items:          all,
		AllSucceeded:   !st.canceled && st.firstFailedPage == 0,
		LastFailedPage: st.firstFailedPage,
		DetectedPages:  st.detectedPages,
	}
}

func (st *pageState) recordFailure() {
	if st.firstFailedPage == 0 {
		st.firstFailedPage = st.page
	}
}

func (st *pageState) detectPages(hooks PageHooks, html string) {
	if st.detectedPages != -1 {
		return
	}
	st.detectedPages = hooks.ParseTotalPages(html)
	if st.detectedPages <= 0 {
		st.detectedPages = 1
	}
}

// advance increments the page and checks boundaries. It returns true when
// the loop should continue, or false when pagination is exhausted.
func (st *pageState) advance(hooks PageHooks) bool {
	st.page++
	if st.detectedPages > 0 && st.page > st.detectedPages {
		if st.mode == ModeFull && st.refreshCount < st.maxRefresh {
			newDetected := hooks.ParseTotalPages(hooks.BuildPageURL(1))
			if newDetected > st.detectedPages {
				slog.Info("Pager: detected page growth, extending scan", "old", st.detectedPages, "new", newDetected)
				st.detectedPages = newDetected
				st.refreshCount++
				return true
			}
		}
		return false
	}
	return true
}

// processItems runs ProcessItems and determines whether the loop should
// continue, break, or return (when failures exist at a terminal state).
func (st *pageState) processItems(hooks PageHooks, items any, all *[]any) processAction {
	pageNew, allKnown := hooks.ProcessItems(items)
	if len(pageNew) > 0 {
		*all = append(*all, pageNew...)
		st.emptyPages = 0
	} else {
		st.emptyPages++
	}

	if allKnown {
		if st.firstFailedPage > 0 {
			return actionReturn
		}
		return actionBreak
	}

	if st.mode == ModeIncremental && st.emptyPages >= st.maxEmpty {
		if st.firstFailedPage > 0 {
			return actionReturn
		}
		return actionBreak
	}

	return actionContinue
}

// Run scrapes pages with retry, dynamic tail refresh, maxEmptyPages break,
// and firstFailedPage tracking.
func (p *DefaultPager) Run(ctx context.Context, hooks PageHooks, opts Options) Result {
	var all []any
	st := newPageState(opts)

	for {
		action := p.processPage(ctx, hooks, st, &all)
		if action == actionReturn {
			return st.result(all)
		}
		if action == actionBreak {
			break
		}
	}

	return st.result(all)
}

// processPage handles one page iteration: scrape, parse, process, advance.
func (p *DefaultPager) processPage(ctx context.Context, hooks PageHooks, st *pageState, all *[]any) processAction {
	if ctx.Err() != nil {
		st.canceled = true
		return actionReturn
	}

	url := hooks.BuildPageURL(st.page)

	html, err := scrapeWithRetry(ctx, hooks, url, st.maxRetries, st.page)
	if err != nil {
		st.recordFailure()
		slog.Error("Pager: scrape failed after max retries", "page", st.page, logutil.LogKeyURL, url, logutil.LogKeyError, err)
		if st.advance(hooks) {
			return actionContinue
		}
		return actionBreak
	}

	st.detectPages(hooks, html)

	items, err := parseWithRetry(ctx, hooks, url, html, st.maxRetries, st.page)
	if err != nil {
		st.recordFailure()
		slog.Error("Pager: parse failed after max retries", "page", st.page, logutil.LogKeyURL, url, logutil.LogKeyError, err)
		if st.advance(hooks) {
			return actionContinue
		}
		return actionBreak
	}

	if items == nil {
		return actionBreak
	}

	action := st.processItems(hooks, items, all)
	if action != actionContinue {
		return action
	}

	if st.advance(hooks) {
		return actionContinue
	}
	return actionBreak
}

// scrapeWithRetry attempts to scrape a URL with up to maxRetries attempts.
func scrapeWithRetry(ctx context.Context, hooks PageHooks, url string, maxRetries int, page int) (string, error) {
	var html string
	var err error
	for attempt := range maxRetries {
		html, err = hooks.RunScraper(url)
		if err == nil {
			return html, nil
		}
		if attempt < maxRetries-1 {
			slog.Warn("Pager: retry after scrape error", "page", page, logutil.LogKeyURL, url, "attempt", attempt+1, logutil.LogKeyError, err)
			if !sleepCtx(ctx, time.Duration(1<<attempt)*time.Second) {
				return "", ctx.Err()
			}
		}
	}
	return html, err
}

// parseWithRetry attempts to parse the HTML with up to maxRetries attempts,
// re-scraping on parse failure.
func parseWithRetry(ctx context.Context, hooks PageHooks, url, html string, maxRetries int, page int) (any, error) {
	var err error
	var items any
	for attempt := range maxRetries {
		items, err = hooks.ParsePage(html)
		if err == nil {
			return items, nil
		}
		if attempt < maxRetries-1 {
			slog.Warn("Pager: retry after parse error", "page", page, logutil.LogKeyURL, url, "attempt", attempt+1, logutil.LogKeyError, err)
			reHTML, reErr := hooks.RunScraper(url)
			if reErr != nil {
				slog.Error("Pager: re-scrape failed after parse error", "page", page, logutil.LogKeyURL, url, logutil.LogKeyError, reErr)
				return nil, reErr
			}
			html = reHTML
		}
	}
	return items, err
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
