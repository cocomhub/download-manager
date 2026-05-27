// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task_test

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"
	"github.com/cocomhub/download-manager/task"
)

func TestBaseTask_ResetZombieState_OnlyResetsDownloading(t *testing.T) {
	bt, err := task.NewBaseTask(&config.Task{
		ID:      "t1",
		Type:    "base",
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
	}, task.Options{})
	if err != nil {
		t.Fatalf("NewBaseTask error: %v", err)
	}

	cases := []struct {
		status string
		want   string
	}{
		{status: dlcore.StatusDownloading, want: dlcore.StatusPending},
		{status: dlcore.StatusPending, want: dlcore.StatusPending},
		{status: dlcore.StatusFailed, want: dlcore.StatusFailed},
		{status: dlcore.StatusCompleted, want: dlcore.StatusCompleted},
		{status: dlcore.StatusCancelled, want: dlcore.StatusCancelled},
	}

	for _, tc := range cases {
		url := "http://example.com/" + tc.status
		obj := &model.DownloadObject{
			TaskID: bt.ID(),
			URL:    url,
			Status: tc.status,
			Extra:  map[string]any{},
		}

		seed := *obj
		if err := bt.Storage().Update(&seed); err != nil {
			t.Fatalf("seed storage err: %v", err)
		}

		bt.ResetZombieState(obj)

		if obj.Status != tc.want {
			t.Fatalf("status=%s: expected obj status %s, got %s", tc.status, tc.want, obj.Status)
		}

		stored, err := bt.Storage().Get(url)
		if err != nil {
			t.Fatalf("get storage err: %v", err)
		}
		if stored == nil {
			t.Fatalf("expected stored obj")
		}
		if stored.Status != tc.want {
			t.Fatalf("status=%s: expected stored status %s, got %s", tc.status, tc.want, stored.Status)
		}
	}
}
