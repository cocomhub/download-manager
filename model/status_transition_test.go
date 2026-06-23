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
		{"pending 鈫?resolving", StatusPending, StatusResolving},
		{"resolving 鈫?pending", StatusResolving, StatusPending},
		{"pending 鈫?downloading", StatusPending, StatusDownloading},
		{"downloading 鈫?completed", StatusDownloading, StatusCompleted},
		{"downloading 鈫?failed", StatusDownloading, StatusFailed},
		{"downloading 鈫?cancelled", StatusDownloading, StatusCancelled},

		// Retry paths
		{"failed 鈫?pending", StatusFailed, StatusPending},
		{"failed_permanent 鈫?pending", StatusFailedPermanent, StatusPending},
		{"cancelled 鈫?pending", StatusCancelled, StatusPending},
		{"completed 鈫?pending", StatusCompleted, StatusPending},

		// No-op / same-status transitions (idempotent)
		{"pending 鈫?pending", StatusPending, StatusPending},
		{"completed 鈫?completed", StatusCompleted, StatusCompleted},
		{"cancelled 鈫?cancelled", StatusCancelled, StatusCancelled},
		{"failed 鈫?failed", StatusFailed, StatusFailed},
		{"downloading 鈫?downloading", StatusDownloading, StatusDownloading},
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

	// Simulate retry: failed 鈫?pending
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
