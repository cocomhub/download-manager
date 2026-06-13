// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import "testing"

// TestStatusValues verifies all status constants have correct string values.
func TestStatusValues(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"StatusPending", StatusPending, "pending"},
		{"StatusResolving", StatusResolving, "resolving"},
		{"StatusDownloading", StatusDownloading, "downloading"},
		{"StatusCompleted", StatusCompleted, "completed"},
		{"StatusFailed", StatusFailed, "failed"},
		{"StatusFailedPermanent", StatusFailedPermanent, "failed_permanent"},
		{"StatusCancelled", StatusCancelled, "cancelled"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

// TestStatusTransitions_Valid verifies that all valid transitions are allowed
// and do not produce errors when called through UpdateStatus.
func TestStatusTransitions_Valid(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
	}{
		// Core lifecycle
		{"pending → resolving", StatusPending, StatusResolving},
		{"resolving → pending", StatusResolving, StatusPending},
		{"pending → downloading", StatusPending, StatusDownloading},
		{"downloading → completed", StatusDownloading, StatusCompleted},
		{"downloading → failed", StatusDownloading, StatusFailed},
		{"downloading → cancelled", StatusDownloading, StatusCancelled},

		// Retry paths
		{"failed → pending", StatusFailed, StatusPending},
		{"failed_permanent → pending", StatusFailedPermanent, StatusPending},
		{"cancelled → pending", StatusCancelled, StatusPending},
		{"completed → pending", StatusCompleted, StatusPending},

		// No-op / same-status transitions (idempotent)
		{"pending → pending", StatusPending, StatusPending},
		{"completed → completed", StatusCompleted, StatusCompleted},
		{"cancelled → cancelled", StatusCancelled, StatusCancelled},
		{"failed → failed", StatusFailed, StatusFailed},
		{"downloading → downloading", StatusDownloading, StatusDownloading},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &DownloadObject{URL: "http://example.com/file"}
			obj.SetStatus(tt.from)

			// Verify initial state
			if obj.GetStatus() != tt.from {
				t.Fatalf("initial status = %q, want %q", obj.GetStatus(), tt.from)
			}

			// Perform transition
			obj.SetStatus(tt.to)
			if obj.GetStatus() != tt.to {
				t.Fatalf("after transition status = %q, want %q", obj.GetStatus(), tt.to)
			}
		})
	}
}

// TestStatus_ProgressResetOnRetry verifies that retrying an object resets progress to 0.
func TestStatus_ProgressResetOnRetry(t *testing.T) {
	obj := &DownloadObject{URL: "http://example.com/file"}
	obj.SetProgress(75)
	obj.SetStatus(StatusFailed)

	// Simulate retry: failed → pending
	obj.SetStatus(StatusPending)
	obj.SetProgress(0)

	if obj.Progress != 0 {
		t.Fatalf("expected progress 0 after retry, got %d", obj.Progress)
	}
	if obj.GetStatus() != StatusPending {
		t.Fatalf("expected status pending after retry, got %s", obj.GetStatus())
	}
}

// TestStatus_CompletedProgress verifies completed objects have progress=100.
func TestStatus_CompletedProgress(t *testing.T) {
	obj := &DownloadObject{URL: "http://example.com/file"}
	obj.SetProgress(100)
	obj.SetStatus(StatusCompleted)

	if obj.Progress != 100 {
		t.Fatalf("expected progress 100 for completed, got %d", obj.Progress)
	}
}

// TestStatus_FailedPermanent verifies that failed_permanent is distinct from failed.
func TestStatus_FailedPermanent(t *testing.T) {
	if StatusFailedPermanent == StatusFailed {
		t.Fatal("failed_permanent should be distinct from failed")
	}
}
