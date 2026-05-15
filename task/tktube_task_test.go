// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import "testing"

func TestCreateObjectFromVideoItem_PersistsTaskTypeMetadata(t *testing.T) {
	tk := &TktubeTask{
		BaseTask: NewBaseTask("t1", "/tmp/save", nil),
	}
	tk.pathStrategy = NewPathStrategy("first_fixed", "/tmp/save")

	obj := tk.createObjectFromVideoItem(videoItem{
		href:     "https://example.com/video/1",
		title:    "【高画质】CLUB-100C",
		duration: "10:00",
		date:     "2026-01-01",
	})

	if got := obj.Metadata["task_type"]; got != TypeTktube {
		t.Fatalf("expect task_type %q, got %q", TypeTktube, got)
	}
}
