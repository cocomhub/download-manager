// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package scrape

import (
	"context"
	"log/slog"
)

// Driver combines Tracker + Pager to provide a single Scrape entry point.
// It reads tracker state, selects mode (full/incremental), runs the pager,
// then updates tracker based on the result.
type Driver struct {
	tracker Tracker
	pager   Pager
}

func NewDriver(tracker Tracker, pager Pager) *Driver {
	return &Driver{tracker: tracker, pager: pager}
}

// Scrape performs a full or incremental scrape as determined by the tracker state.
// It returns the scraped items and whether the run was successful.
func (d *Driver) Scrape(ctx context.Context, taskID string, hooks PageHooks, baseOpts Options) Result {
	// Determine mode from tracker state
	var opts Options
	if baseOpts.MaxRetries <= 0 {
		opts.MaxRetries = 3
	} else {
		opts.MaxRetries = baseOpts.MaxRetries
	}
	if baseOpts.MaxEmptyPages <= 0 {
		opts.MaxEmptyPages = 3
	} else {
		opts.MaxEmptyPages = baseOpts.MaxEmptyPages
	}
	if baseOpts.MaxTailRefresh <= 0 {
		opts.MaxTailRefresh = 5
	} else {
		opts.MaxTailRefresh = baseOpts.MaxTailRefresh
	}

	if d.tracker.IsFullSucceeded(taskID) {
		// Incremental mode
		opts.Mode = ModeIncremental
		opts.StartPage = 1
		slog.Debug("ScrapeDriver: incremental mode", "task_id", taskID)
	} else {
		// Full mode: check progress for resume
		opts.Mode = ModeFull
		if info, ok := d.tracker.LoadProgress(taskID); ok && info.LastFailedPage > 0 {
			opts.StartPage = info.LastFailedPage
			slog.Info("ScrapeDriver: resuming full scan from page", "task_id", taskID, "from", info.LastFailedPage, "max_detected", info.MaxDetectedPage)
		} else {
			opts.StartPage = 1
			slog.Debug("ScrapeDriver: cold start full scan", "task_id", taskID)
		}
	}

	// Run the pager
	result := d.pager.Run(ctx, hooks, opts)

	// Update tracker based on result
	if result.AllSucceeded {
		if opts.Mode == ModeFull {
			// Full scan succeeded -> mark full succ + clear progress
			if err := d.tracker.MarkFullSucceeded(taskID); err != nil {
				slog.Error("ScrapeDriver: failed to mark full succeeded", "task_id", taskID, "error", err)
			} else {
				slog.Info("ScrapeDriver: full scan succeeded", "task_id", taskID, "pages", result.DetectedPages)
			}
			if err := d.tracker.ClearProgress(taskID); err != nil {
				slog.Warn("ScrapeDriver: failed to clear progress", "task_id", taskID, "error", err)
			}
		}
		// Incremental success: keep full_succ marker intact (do nothing)
	} else {
		// Failure: save progress, delete full_succ (if was incremetal)
		pi := ProgressInfo{
			LastFailedPage:  result.LastFailedPage,
			MaxDetectedPage: result.DetectedPages,
		}
		if err := d.tracker.SaveProgress(taskID, pi); err != nil {
			slog.Error("ScrapeDriver: failed to save progress", "task_id", taskID, "error", err)
		}
		if opts.Mode == ModeIncremental {
			// Incremental failure: downgrade, next run will be full
			if err := d.tracker.DeleteFullSuccess(taskID); err != nil {
				slog.Error("ScrapeDriver: failed to downgrade full_succ", "task_id", taskID, "error", err)
			} else {
				slog.Warn("ScrapeDriver: incremental failed, downgraded to full", "task_id", taskID, "failed_page", result.LastFailedPage)
			}
		}
	}

	return result
}
