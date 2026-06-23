// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/model"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestFailedCount_TypeAssertion verifies that if failedCount contains a
// non-*atomic.Int64 value, the download path handles it gracefully (no panic)
// instead of panic-ing with a type assertion error.
//
// A production fix was applied to download.go:161 to safely guard the type assertion.
func TestFailedCount_TypeAssertion(t *testing.T) {
	mgr, _ := newMockManager(t, "failed-type", 1,
		mockdl.New(mockdl.ModeAlwaysFail))
	startManager(t, mgr)

	task := waitForTask(t, mgr, "failed-type")

	// Inject a bad value into failedCount to simulate corruption.
	url := "http://mock-download/file-0.bin"
	mgr.failedCount.Store(url, "bad_string_value")

	// The download should handle gracefully without panic.
	// After max retries, the object is marked as failed_permanent by MarkAsFailed.
	waitForObjectsFinal(t, mgr, task, 1, model.StatusFailedPermanent, 5*time.Second)

	// Verify the object reached a permanent-failure state without causing a panic.
	all := getAllObjectsFromTask(t, task)
	for _, obj := range all {
		if obj.GetStatus() != model.StatusFailedPermanent {
			t.Errorf("expected StatusFailedPermanent, got %s for %s", obj.GetStatus(), obj.URL)
		}
	}

	// Verify the old bad value was replaced by a proper atomic.
	v, ok := mgr.failedCount.Load(url)
	if !ok {
		t.Fatal("expected failedCount to have entry after test")
	}
	if _, ok := v.(*atomic.Int64); !ok {
		t.Errorf("expected *atomic.Int64 in failedCount, got %T", v)
	}
}
