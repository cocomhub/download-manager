// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import "github.com/cocomhub/download-manager/model"

// SiteAdapter defines the site-specific pagination scraping, object building,
// and detail resolution behaviors needed by PagingScanner.
//
// Each task type (tktube/hanime/vikacg) provides its own implementation,
// bridging existing internal methods to this interface without requiring
// those methods to be exported or restructured.
type SiteAdapter interface {
	// --- Pagination lifecycle (maps to scrape.PageHooks) ---

	// BuildPageURL constructs the URL for the given page (1-based).
	BuildPageURL(page int) string

	// RunScraper fetches page content (HTML or JSON) from the given URL.
	// For API-based tasks (e.g. vikacg POST), this method encodes the
	// page number into the URL string and decodes it here.
	RunScraper(url string) (string, error)

	// ParseTotalPages extracts total page count from the first page content.
	// Returns ≤ 0 when unknown (e.g. API-driven pagination without total).
	ParseTotalPages(html string) int

	// ParsePage extracts structured items from page content.
	// The return value is a site-specific slice (e.g. []videoItem, []hanimeItem).
	ParsePage(html string) (any, error)

	// --- Item → DownloadObject conversion ---

	// ItemsToURLs extracts dedup-required URLs from parsed items, preserving order.
	// The returned slice must have the same length as the number of items.
	ItemsToURLs(items any) []string

	// BuildObject constructs a DownloadObject for the item at the given index.
	// The items parameter is the same value returned by ParsePage;
	// implementations type-assert it to their concrete slice type.
	BuildObject(items any, index int) (*model.DownloadObject, error)

	// --- Detail resolution (optional) ---

	// ResolveDetail fetches and populates detailed fields on obj.
	// For tasks where list page ≡ detail page (e.g. vikacg), return nil.
	// Return an error when the object should be skipped.
	ResolveDetail(obj *model.DownloadObject) error

	// GetDownloadHeaders returns HTTP headers used during download.
	GetDownloadHeaders() map[string]string
}