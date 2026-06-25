// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package scrape

import (
	"context"
	"log/slog"

	"github.com/cocomhub/download-manager/pkg/logutil"
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
	opts := applyDefaultOptions(baseOpts)
	d.applyScrapeMode(taskID, &opts)

	result := d.pager.Run(ctx, hooks, opts)

	d.updateTracker(taskID, result, opts)

	return result
}

// applyDefaultOptions fills in default values for unset option fields.
func applyDefaultOptions(baseOpts Options) Options {
	opts := baseOpts
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = 3
	}
	if opts.MaxEmptyPages <= 0 {
		opts.MaxEmptyPages = 3
	}
	if opts.MaxTailRefresh <= 0 {
		opts.MaxTailRefresh = 5
	}
	return opts
}

// applyScrapeMode determines the scrape mode (full/incremental) and start page
// from the tracker state, mutating opts in place.
func (d *Driver) applyScrapeMode(taskID string, opts *Options) {
	if d.tracker.IsFullSucceeded(taskID) {
		opts.Mode = ModeIncremental
		opts.StartPage = 1
		slog.Debug("ScrapeDriver: incremental mode", logutil.LogKeyTaskID, taskID)
		return
	}
	opts.Mode = ModeFull
	if info, ok := d.tracker.LoadProgress(taskID); ok && info.LastFailedPage > 0 {
		opts.StartPage = info.LastFailedPage
		slog.Info("ScrapeDriver: resuming full scan from page", logutil.LogKeyTaskID, taskID, "from", info.LastFailedPage, "max_detected", info.MaxDetectedPage)
		return
	}
	opts.StartPage = 1
	slog.Debug("ScrapeDriver: cold start full scan", logutil.LogKeyTaskID, taskID)
}

// updateTracker persists the result of a scrape run to the tracker.
func (d *Driver) updateTracker(taskID string, result Result, opts Options) {
	if !result.AllSucceeded {
		d.saveFailureAndMaybeDowngrade(taskID, result, opts)
		return
	}
	if opts.Mode != ModeFull {
		return
	}
	d.markFullSuccess(taskID, result)
}

// saveFailureAndMaybeDowngrade persists progress info and, if the failed run
// was incremental, downgrades the full_succ marker so next run is full.
func (d *Driver) saveFailureAndMaybeDowngrade(taskID string, result Result, opts Options) {
	pi := ProgressInfo{
		LastFailedPage:  result.LastFailedPage,
		MaxDetectedPage: result.DetectedPages,
	}
	if err := d.tracker.SaveProgress(taskID, pi); err != nil {
		slog.Error("ScrapeDriver: failed to save progress", logutil.LogKeyTaskID, taskID, logutil.LogKeyError, err)
	}
	if opts.Mode != ModeIncremental {
		return
	}
	if err := d.tracker.DeleteFullSuccess(taskID); err != nil {
		slog.Error("ScrapeDriver: failed to downgrade full_succ", logutil.LogKeyTaskID, taskID, logutil.LogKeyError, err)
	} else {
		slog.Warn("ScrapeDriver: incremental failed, downgraded to full", logutil.LogKeyTaskID, taskID, "failed_page", result.LastFailedPage)
	}
}

// markFullSuccess records a successful full scan and clears progress.
func (d *Driver) markFullSuccess(taskID string, result Result) {
	if err := d.tracker.MarkFullSucceeded(taskID); err != nil {
		slog.Error("ScrapeDriver: failed to mark full succeeded", logutil.LogKeyTaskID, taskID, logutil.LogKeyError, err)
	} else {
		slog.Info("ScrapeDriver: full scan succeeded", logutil.LogKeyTaskID, taskID, "pages", result.DetectedPages)
	}
	if err := d.tracker.ClearProgress(taskID); err != nil {
		slog.Warn("ScrapeDriver: failed to clear progress", logutil.LogKeyTaskID, taskID, logutil.LogKeyError, err)
	}
}
