// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package core

import "context"

// PathStrategy defines how save paths are resolved for download objects.
type PathStrategy interface {
	Resolve(baseDir string, taskID string, title string, fileType string) (string, string)
}

// ScrapeCap is implemented by tasks that support Manager-driven page scraping.
// Manager's scan loop calls Scrape(ctx) periodically to discover new objects.
// Implementations MUST honor ctx cancellation to allow timely shutdown.
type ScrapeCap interface {
	Scrape(ctx context.Context) error
}
