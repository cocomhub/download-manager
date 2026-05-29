// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package scrape

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileTracker_FullSuccess(t *testing.T) {
	dir := t.TempDir()
	tracker := NewFileTracker(dir)

	taskID := "test-task-1"

	if tracker.IsFullSucceeded(taskID) {
		t.Fatal("Should not be full succeeded initially")
	}

	if err := tracker.MarkFullSucceeded(taskID); err != nil {
		t.Fatal("MarkFullSucceeded failed:", err)
	}

	if !tracker.IsFullSucceeded(taskID) {
		t.Fatal("Should be full succeeded after MarkFullSucceeded")
	}

	// Verify file exists on disk
	succPath := filepath.Join(dir, "full_succ", taskID+".json")
	if _, err := os.Stat(succPath); err != nil {
		t.Fatal("Expected succ file on disk:", err)
	}

	// Delete and verify
	if err := tracker.DeleteFullSuccess(taskID); err != nil {
		t.Fatal("DeleteFullSuccess failed:", err)
	}
	if tracker.IsFullSucceeded(taskID) {
		t.Fatal("Should not be full succeeded after DeleteFullSuccess")
	}
}

func TestFileTracker_Progress(t *testing.T) {
	dir := t.TempDir()
	tracker := NewFileTracker(dir)

	taskID := "test-task-2"

	// No progress initially
	_, ok := tracker.LoadProgress(taskID)
	if ok {
		t.Fatal("Should not have progress initially")
	}

	info := ProgressInfo{LastFailedPage: 5, MaxDetectedPage: 10}
	if err := tracker.SaveProgress(taskID, info); err != nil {
		t.Fatal("SaveProgress failed:", err)
	}

	loaded, ok := tracker.LoadProgress(taskID)
	if !ok {
		t.Fatal("Should have progress after SaveProgress")
	}
	if loaded.LastFailedPage != 5 {
		t.Fatalf("Expected LastFailedPage=5, got %d", loaded.LastFailedPage)
	}
	if loaded.MaxDetectedPage != 10 {
		t.Fatalf("Expected MaxDetectedPage=10, got %d", loaded.MaxDetectedPage)
	}

	// Clear and verify
	if err := tracker.ClearProgress(taskID); err != nil {
		t.Fatal("ClearProgress failed:", err)
	}
	_, ok = tracker.LoadProgress(taskID)
	if ok {
		t.Fatal("Should not have progress after ClearProgress")
	}
}
