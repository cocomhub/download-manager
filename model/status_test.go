// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import "testing"

func TestStatusResolving(t *testing.T) {
	if StatusResolving == "" {
		t.Error("StatusResolving should not be empty")
	}

	// 纭繚涓庡叾浠栫姸鎬佸€间笉鍐茬獊
	statuses := []string{StatusPending, StatusDownloading, StatusCompleted, StatusFailed, StatusFailedPermanent, StatusCancelled}
	for _, s := range statuses {
		if s == StatusResolving {
			t.Errorf("StatusResolving %q conflicts with existing status %q", StatusResolving, s)
		}
	}
}

func TestStatusResolvingValue(t *testing.T) {
	if StatusResolving != "resolving" {
		t.Errorf("expected StatusResolving = %q, got %q", "resolving", StatusResolving)
	}
}
