// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tktube

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/task"
)

func TestCreateObjectFromVideoItem_PersistsTaskTypeMetadata(t *testing.T) {
	tk, err := task.NewTask(&config.Task{
		ID:      "t1",
		Type:    TaskType,
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{
			Type: "memory",
		},
		Extra: nil,
	})
	if err != nil {
		t.Fatalf("new task err: %s", err)
	}

	obj := tk.(*Task).createObjectFromVideoItem(videoItem{
		href:     "https://example.com/video/1",
		title:    "【高画质】CLUB-100C",
		duration: "10:00",
		date:     "2026-01-01",
	})

	if got := obj.Metadata["task_type"]; got != TaskType {
		t.Fatalf("expect task_type %q, got %q", TaskType, got)
	}
}

// TestCountActiveDownloads_RuntimeOnly verifies that countActiveDownloads
// counts only from runtime objects and does not depend on storage queries.
func TestCountActiveDownloads_RuntimeOnly(t *testing.T) {
	tk, err := task.NewTask(&config.Task{
		ID:      "t2",
		Type:    TaskType,
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{
			Type: "memory",
		},
		Extra: nil,
	})
	if err != nil {
		t.Fatalf("new task err: %s", err)
	}

	tt := tk.(*Task)

	// Store some objects in storage with StatusDownloading to verify
	// that countActiveDownloads does NOT count them from storage.
	tt.Storage().Update(&model.DownloadObject{
		URL:    "http://test/storage-downloading",
		Status: model.StatusDownloading,
	})
	tt.Storage().Update(&model.DownloadObject{
		URL:    "http://test/storage-pending",
		Status: model.StatusPending,
	})

	// Create runtime objects with mixed statuses
	runtimeObjects := []*model.DownloadObject{
		{URL: "http://test/runtime-downloading-1", Status: model.StatusDownloading},
		{URL: "http://test/runtime-downloading-2", Status: model.StatusDownloading},
		{URL: "http://test/runtime-pending", Status: model.StatusPending},
		{URL: "http://test/runtime-completed", Status: model.StatusCompleted},
		{URL: "http://test/runtime-failed", Status: model.StatusFailed},
		{URL: "http://test/runtime-cancelled", Status: model.StatusCancelled},
	}

	active := tt.countActiveDownloads(runtimeObjects)
	if active != 2 {
		t.Errorf("expected 2 active downloads from runtime objects, got %d", active)
	}
}
