// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/model"
)

// TestApplyGroupPriorityPolicies_NonTktubeNoop verifies that
// applyGroupPriorityPolicies is a no-op for non-tktube task types.
func TestApplyGroupPriorityPolicies_NonTktubeNoop(t *testing.T) {
	store := &memStore{m: make(map[string]*model.DownloadObject)}
	obj := &model.DownloadObject{
		TaskID: "non-tktask",
		URL:    "u1",
		Metadata: map[string]string{
			"title":         "Some video",
			"task_type":     "other-type",
			"content_group": "group-1",
		},
		Status: model.StatusCompleted,
	}
	store.Update(obj)

	task := &fakeTktTask{
		id:  "non-tktask",
		typ: "some-other-type",
		st:  store,
	}

	mgr := &Manager{}
	// Should not panic or modify anything for non-tktube types.
	mgr.applyGroupPriorityPolicies(task, obj)

	// Object should still be completed (no side effects).
	if got := obj.GetStatus(); got != model.StatusCompleted {
		t.Errorf("expected StatusCompleted for non-tktube task, got %s", got)
	}
}
