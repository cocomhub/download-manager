// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tktube

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
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
