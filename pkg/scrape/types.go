// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package scrape

import "context"

// Mode represents the scraping mode: full or incremental.
type Mode int

const (
	ModeFull Mode = iota
	ModeIncremental
)

// PageHooks contains all callbacks needed to scrape a paginated site.
type PageHooks struct {
	BuildPageURL    func(page int) string
	RunScraper      func(url string) (string, error)
	ParseTotalPages func(html string) int
	ParsePage       func(html string) (any, error)
	ProcessItems    func(items any) (newItems []any, allKnown bool)
}

// Options configures a scraping run.
type Options struct {
	Mode           Mode
	StartPage      int // resume from this page; 1 for cold start
	MaxEmptyPages  int // only for Incremental; stop after N empty pages, default 3
	MaxRetries     int // retries per page scrape/parse, default 3
	MaxTailRefresh int // only for Full; max times to refresh totalPages when page reaches end, default 5
}

// Result holds the outcome of a scraping run.
type Result struct {
	Items          []any
	AllSucceeded   bool
	LastFailedPage int // >0 indicates failure at this page
	DetectedPages  int // final detected page count after dynamic refresh
}

// Pager iterates pages using hooks and options.
type Pager interface {
	Run(ctx context.Context, hooks PageHooks, opts Options) Result
}

// ProgressInfo is persisted when a full scan is interrupted (partial failure).
type ProgressInfo struct {
	LastFailedPage  int `json:"last_failed_page"`
	MaxDetectedPage int `json:"max_detected_page"`
}

// SuccessInfo is the content of the full_succ marker file.
type SuccessInfo struct {
}
