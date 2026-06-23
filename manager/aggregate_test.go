// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
)

func TestAggregateTypes_CaseInsensitiveAndPrefix(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{ID: "t1", Type: "hanime_search"},
			{ID: "t2", Type: "vikacg"},
		},
	}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)
	t1 := &mockTask{
		id:  "t1",
		typ: "hanime_search",
		objs: []*model.DownloadObject{
			{TaskID: "t1", URL: "u1", Metadata: map[string]string{"title": "a", "date": ""}},
		},
	}
	t2 := &mockTask{
		id:  "t2",
		typ: "vikacg",
		objs: []*model.DownloadObject{
			{TaskID: "t2", URL: "u2", Metadata: map[string]string{"title": "b", "date": ""}},
		},
	}
	m.tasks.Store("t1", t1)
	m.tasks.Store("t2", t2)

	res1, err := m.AggregateObjects(1, -1, "", "", "", []string{"HANIME"})
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}
	objs1, ok := res1["objects"].([]*model.DownloadObject)
	if !ok {
		t.Fatalf("unexpected objects type")
	}
	if len(objs1) != 1 || objs1[0].TaskID != "t1" {
		t.Fatalf("expect only t1 matched, got: %+v", objs1)
	}

	res2, err := m.AggregateObjects(1, -1, "", "", "", []string{"vikACG"})
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}
	objs2, ok := res2["objects"].([]*model.DownloadObject)
	if !ok {
		t.Fatalf("expected objects slice, got %T", res2["objects"])
	}
	if len(objs2) != 1 || objs2[0].TaskID != "t2" {
		t.Fatalf("expect only t2 matched, got: %+v", objs2)
	}

	res3, err := m.AggregateObjects(1, -1, "", "", "", []string{})
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}
	objs3, ok := res3["objects"].([]*model.DownloadObject)
	if !ok {
		t.Fatalf("expected objects slice, got %T", res3["objects"])
	}
	if len(objs3) != 2 {
		t.Fatalf("expect 2 objects when no type filter, got: %d", len(objs3))
	}
}
