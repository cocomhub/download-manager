// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
)

func TestAggregateByContent_SelectRepresentativeAndSize(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{ID: "t1", Type: "tktube"},
		},
	}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)

	// Group A: one HQ and one C
	o1 := &model.DownloadObject{
		TaskID: "t1", URL: "u1",
		Metadata: map[string]string{"title": "【高画质】CLUB-100", "content_group": "CLUB-100", "date": "2024-01-01"},
		Extra:    map[string]any{},
	}
	o2 := &model.DownloadObject{
		TaskID: "t1", URL: "u2",
		Metadata: map[string]string{"title": "CLUB-100C", "content_group": "CLUB-100", "date": "2024-01-02"},
		Extra:    map[string]any{},
	}
	// Group B: single item no flags
	o3 := &model.DownloadObject{
		TaskID: "t1", URL: "u3",
		Metadata: map[string]string{"title": "ABP-456", "content_group": "ABP-456", "date": "2024-02-01"},
		Extra:    map[string]any{},
	}
	t1 := &mockTask{
		id:   "t1",
		typ:  "tktube",
		objs: []*model.DownloadObject{o1, o2, o3},
	}
	m.tasks.Store("t1", t1)

	res, err := m.AggregateByContent(1, -1, "", "date_desc", "all", []string{})
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}
	objs, ok := res["objects"].([]*model.DownloadObject)
	if !ok {
		t.Fatalf("expected objects slice, got %T", res["objects"])
	}
	if len(objs) != 2 {
		t.Fatalf("expect 2 groups, got %d", len(objs))
	}
	// Find group CLUB-100 representative should be o1 (has HQ)
	var repA *model.DownloadObject
	var repB *model.DownloadObject
	for _, o := range objs {
		if o.Metadata[model.MetadataKeyContentGroup] == "CLUB-100" {
			repA = o
		}
		if o.Metadata[model.MetadataKeyContentGroup] == "ABP-456" {
			repB = o
		}
	}
	if repA == nil || repA.URL != "u1" {
		t.Fatalf("expect CLUB-100 representative u1, got %+v", repA)
	}
	if repA.Extra["group_size"] != 2 {
		t.Fatalf("expect group_size 2 for CLUB-100, got %v", repA.Extra["group_size"])
	}
	if repB == nil || repB.URL != "u3" {
		t.Fatalf("expect ABP-456 representative u3, got %+v", repB)
	}
	if repB.Extra["group_size"] != 1 {
		t.Fatalf("expect group_size 1 for ABP-456, got %v", repB.Extra["group_size"])
	}
}

func TestAggregateByContent_ScopesSameGroupAcrossTasks(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{ID: "t1", Type: "tktube"},
			{ID: "t2", Type: "tktube"},
		},
	}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)

	t1Obj := &model.DownloadObject{
		TaskID: "t1",
		URL:    "u1",
		Metadata: map[string]string{
			"title":         "CLUB-100",
			"content_group": "CLUB-100",
			"date":          "2024-01-01",
		},
		Extra: map[string]any{},
	}
	t2Obj := &model.DownloadObject{
		TaskID: "t2",
		URL:    "u2",
		Metadata: map[string]string{
			"title":         "CLUB-100",
			"content_group": "CLUB-100",
			"date":          "2024-01-02",
		},
		Extra: map[string]any{},
	}
	m.tasks.Store("t1", &mockTask{id: "t1", typ: "tktube", objs: []*model.DownloadObject{t1Obj}})
	m.tasks.Store("t2", &mockTask{id: "t2", typ: "tktube", objs: []*model.DownloadObject{t2Obj}})

	res, err := m.AggregateByContent(1, -1, "", "date_desc", "all", []string{})
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}
	objs, ok := res["objects"].([]*model.DownloadObject)
	if !ok {
		t.Fatalf("expected objects slice, got %T", res["objects"])
	}
	if len(objs) != 2 {
		t.Fatalf("expect 2 scoped groups, got %d", len(objs))
	}

	got := m.GetObjectsByScopedGroup("t1", "tktube", "CLUB-100")
	if len(got) != 1 || got[0].TaskID != "t1" {
		t.Fatalf("expect only t1 object, got %+v", got)
	}
}

func TestAggregateByContent_UnknownKeysStaySeparated(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{ID: "t1", Type: "tktube"},
		},
	}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)

	o1 := &model.DownloadObject{
		TaskID: "t1",
		URL:    "u1",
		Metadata: map[string]string{
			"title":         "随机标题甲",
			"content_group": "unknown+随机标题甲",
			"date":          "2024-01-01",
		},
		Extra: map[string]any{},
	}
	o2 := &model.DownloadObject{
		TaskID: "t1",
		URL:    "u2",
		Metadata: map[string]string{
			"title":         "随机标题乙",
			"content_group": "unknown+随机标题乙",
			"date":          "2024-01-02",
		},
		Extra: map[string]any{},
	}
	m.tasks.Store("t1", &mockTask{id: "t1", typ: "tktube", objs: []*model.DownloadObject{o1, o2}})

	res, err := m.AggregateByContent(1, -1, "", "date_desc", "all", []string{})
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}
	objs, ok := res["objects"].([]*model.DownloadObject)
	if !ok {
		t.Fatalf("expected objects slice, got %T", res["objects"])
	}
	if len(objs) != 2 {
		t.Fatalf("expect 2 unknown groups, got %d", len(objs))
	}
}
