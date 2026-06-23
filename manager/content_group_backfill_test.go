// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"maps"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/titlegroup"
	"github.com/cocomhub/download-manager/storage"
)

type snapshotStore struct {
	m       map[string]*model.DownloadObject
	updates int
}

func cloneDownloadObject(obj *model.DownloadObject) *model.DownloadObject {
	if obj == nil {
		return nil
	}
	cp := &model.DownloadObject{
		TaskID:   obj.TaskID,
		URL:      obj.URL,
		SavePath: obj.SavePath,
		Status:   obj.GetStatus(),
		Progress: obj.GetProgress(),
	}
	if obj.Metadata != nil {
		cp.Metadata = make(map[string]string, len(obj.Metadata))
		maps.Copy(cp.Metadata, obj.Metadata)
	}
	if obj.Extra != nil {
		cp.Extra = make(map[string]any, len(obj.Extra))
		maps.Copy(cp.Extra, obj.Extra)
	}
	return cp
}

func (s *snapshotStore) Get(id string) (*model.DownloadObject, error) {
	return cloneDownloadObject(s.m[id]), nil
}

func (s *snapshotStore) Update(obj *model.DownloadObject) error {
	if s.m == nil {
		s.m = make(map[string]*model.DownloadObject)
	}
	s.m[obj.URL] = cloneDownloadObject(obj)
	s.updates++
	return nil
}

func (s *snapshotStore) Delete(id string) error {
	delete(s.m, id)
	return nil
}

func (s *snapshotStore) Search(query *core.StorageQuery) ([]*model.DownloadObject, error) {
	out := make([]*model.DownloadObject, 0, len(s.m))
	for _, obj := range s.m {
		out = append(out, cloneDownloadObject(obj))
	}
	return storage.ApplyQueryToObjects(out, query), nil
}

func (s *snapshotStore) Count(query *core.StorageQuery) (int64, error) {
	out := make([]*model.DownloadObject, 0, len(s.m))
	for _, obj := range s.m {
		out = append(out, cloneDownloadObject(obj))
	}
	return storage.CountObjects(out, query), nil
}

func (s *snapshotStore) Exists(ids []string) (map[string]bool, error) {
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		_, ok := s.m[id]
		result[id] = ok
	}
	return result, nil
}

func TestBackfillContentGroups_RecomputesAndCorrectsSavedValue(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{ID: "t1", Type: "tktube"},
		},
	}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)

	obj := &model.DownloadObject{
		TaskID: "t1",
		URL:    "https://example.com/video/1",
		Metadata: map[string]string{
			"title":         "銆愰珮鐢昏川銆慍LUB-100C",
			"content_group": "WRONG-GROUP",
			"task_type":     core.TaskTypeHanime,
		},
	}
	missingTaskType := &model.DownloadObject{
		TaskID: "t1",
		URL:    "https://example.com/video/2",
		Metadata: map[string]string{
			"title":         "ABP-123",
			"content_group": titlegroup.TKTContentGroupKey("ABP-123", "https://example.com/video/2"),
		},
	}
	store := &snapshotStore{m: map[string]*model.DownloadObject{
		obj.URL:             cloneDownloadObject(obj),
		missingTaskType.URL: cloneDownloadObject(missingTaskType),
	}}
	m.tasks.Store("t1", &fakeTktTask{id: "t1", st: store, objs: []*model.DownloadObject{obj, missingTaskType}})

	m.BackfillContentGroups()

	want := titlegroup.TKTContentGroupKey(obj.Metadata[model.MetadataKeyTitle], obj.URL)
	got := store.m[obj.URL].Metadata[model.MetadataKeyContentGroup]
	if got != want {
		t.Fatalf("expect corrected content_group %q, got %q", want, got)
	}
	if gotType := store.m[obj.URL].Metadata["task_type"]; gotType != core.TaskTypeTktube {
		t.Fatalf("expect corrected task_type %q, got %q", core.TaskTypeTktube, gotType)
	}
	if gotType := store.m[missingTaskType.URL].Metadata["task_type"]; gotType != core.TaskTypeTktube {
		t.Fatalf("expect missing task_type backfilled to %q, got %q", core.TaskTypeTktube, gotType)
	}
	if store.updates != 2 {
		t.Fatalf("expect 2 persisted updates, got %d", store.updates)
	}
}
