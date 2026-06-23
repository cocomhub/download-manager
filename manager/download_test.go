// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"
	"time"

	"github.com/cocomhub/download-manager/model"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestDownload_ExhaustsMaxRetries verifies that objects exhaust all retries and reach StatusFailedPermanent.
func TestDownload_ExhaustsMaxRetries(t *testing.T) {
	mgr, _ := newMockManager(t, "task-maxretry", 1, mockdl.New(mockdl.ModeAlwaysFail))
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "task-maxretry")

	// Wait for the object to be processed 鈥?it should go through all retries
	// MaxRetries is 2 in newMockManager, so after 2 failures it should be permanent
	waitForObjectsFinal(t, mgr, task, 1, model.StatusFailedPermanent, 10*time.Second)
}

// TestDownload_SucceedsBeforeMaxRetries verifies first-fail-then-success works with retries.
func TestDownload_SucceedsBeforeMaxRetries(t *testing.T) {
	mgr, _ := newMockManager(t, "task-retry-ok", 1, mockdl.New(mockdl.ModeFirstFailThenSuccess))
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "task-retry-ok")

	waitForObjectsFinal(t, mgr, task, 1, model.StatusCompleted, 10*time.Second)
}
