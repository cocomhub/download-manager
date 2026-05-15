// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"
	"github.com/cocomhub/download-manager/storage"
	"github.com/cocomhub/download-manager/task"
)

type memStore struct {
	m map[string]*model.DownloadObject
}

func (s *memStore) Get(id string) (*model.DownloadObject, error) { return s.m[id], nil }
func (s *memStore) Update(obj *model.DownloadObject) error {
	if s.m == nil {
		s.m = make(map[string]*model.DownloadObject)
	}
	s.m[obj.URL] = obj
	return nil
}
func (s *memStore) Delete(id string) error { delete(s.m, id); return nil }
func (s *memStore) Search(query *core.StorageQuery) ([]*model.DownloadObject, error) {
	var list []*model.DownloadObject
	for _, o := range s.m {
		list = append(list, o)
	}
	return storage.ApplyQueryToObjects(list, query), nil
}
func (s *memStore) Count(query *core.StorageQuery) (int, error) {
	var list []*model.DownloadObject
	for _, o := range s.m {
		list = append(list, o)
	}
	return storage.CountObjects(list, query), nil
}
func (s *memStore) Exists(ids []string) (map[string]bool, error) {
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		_, ok := s.m[id]
		result[id] = ok
	}
	return result, nil
}

type fakeTktTask struct {
	id   string
	typ  string
	objs []*model.DownloadObject
	dl   core.Downloader
	st   core.Storage
}

func (f *fakeTktTask) ID() string                            { return f.id }
func (f *fakeTktTask) SetDownloader(dl core.Downloader)      { f.dl = dl }
func (f *fakeTktTask) GetDownloadHeaders() map[string]string { return map[string]string{} }
func (f *fakeTktTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	return []*model.DownloadObject{}, nil
}
func (f *fakeTktTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	obj.Status = status
	return f.st.Update(obj)
}
func (f *fakeTktTask) Type() string {
	if f.typ != "" {
		return f.typ
	}
	return task.TypeTktube
}
func (f *fakeTktTask) Close() error                           { return nil }
func (f *fakeTktTask) GetAllObjects() []*model.DownloadObject { return f.objs }
func (f *fakeTktTask) GetStorage() core.Storage               { return f.st }

func TestApplyGroupPriorityPolicies_CancelLowerPriority(t *testing.T) {
	m := &Manager{}
	store := &memStore{m: make(map[string]*model.DownloadObject)}
	// Canonical: hasHQ
	o1 := &model.DownloadObject{
		TaskID: "t1",
		URL:    "u1",
		Metadata: map[string]string{
			"title":         "【高画质】CLUB-100",
			"task_type":     task.TypeTktube,
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusCompleted,
	}
	// Lower priority: hasC only
	o2 := &model.DownloadObject{
		TaskID: "t1",
		URL:    "u2",
		Metadata: map[string]string{
			"title":         "CLUB-100C",
			"task_type":     task.TypeTktube,
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusPending,
	}
	_ = store.Update(o1)
	_ = store.Update(o2)
	task := &fakeTktTask{id: "t1", st: store, objs: []*model.DownloadObject{o1, o2}}

	m.applyGroupPriorityPolicies(task, o1)

	if o2.Status != dlcore.StatusCancelled {
		t.Fatalf("expect o2 cancelled, got %s", o2.Status)
	}
	if o2.Extra["redirect_url"] != o1.URL {
		t.Fatalf("expect redirect_url=%s, got %v", o1.URL, o2.Extra["redirect_url"])
	}
}

func TestApplyGroupPriorityPolicies_SkipWhenSamePriorityConflicts(t *testing.T) {
	m := &Manager{}
	store := &memStore{m: make(map[string]*model.DownloadObject)}

	canonical := &model.DownloadObject{
		TaskID: "t1",
		URL:    "u1",
		Metadata: map[string]string{
			"title":         "【高画质】CLUB-100",
			"task_type":     task.TypeTktube,
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusCompleted,
	}
	p1 := &model.DownloadObject{
		TaskID: "t1",
		URL:    "u2",
		Metadata: map[string]string{
			"title":         "PLAIN-ONE",
			"task_type":     task.TypeTktube,
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusPending,
	}
	p2 := &model.DownloadObject{
		TaskID: "t1",
		URL:    "u3",
		Metadata: map[string]string{
			"title":         "PLAIN-TWO",
			"task_type":     task.TypeTktube,
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusPending,
	}
	_ = store.Update(canonical)
	_ = store.Update(p1)
	_ = store.Update(p2)
	task := &fakeTktTask{id: "t1", st: store, objs: []*model.DownloadObject{canonical, p1, p2}}

	m.applyGroupPriorityPolicies(task, canonical)

	if p1.Status != dlcore.StatusPending || p2.Status != dlcore.StatusPending {
		t.Fatalf("expect conflict group to skip auto-cancel, got %s and %s", p1.Status, p2.Status)
	}
}

func TestApplyGroupPriorityPolicies_DoesNotCancelDownloading(t *testing.T) {
	m := &Manager{}
	store := &memStore{m: make(map[string]*model.DownloadObject)}

	canonical := &model.DownloadObject{
		TaskID: "t1",
		URL:    "u1",
		Metadata: map[string]string{
			"title":         "【高画质】CLUB-100",
			"task_type":     task.TypeTktube,
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusCompleted,
	}
	downloading := &model.DownloadObject{
		TaskID: "t1",
		URL:    "u2",
		Metadata: map[string]string{
			"title":         "CLUB-100C",
			"task_type":     task.TypeTktube,
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusDownloading,
	}
	_ = store.Update(canonical)
	_ = store.Update(downloading)
	task := &fakeTktTask{id: "t1", st: store, objs: []*model.DownloadObject{canonical, downloading}}

	m.applyGroupPriorityPolicies(task, canonical)

	if downloading.Status != dlcore.StatusDownloading {
		t.Fatalf("expect downloading object unchanged, got %s", downloading.Status)
	}
}

func TestApplyGroupPriorityPolicies_ScopesToCurrentTaskWithinSharedStorage(t *testing.T) {
	m := &Manager{}
	store := &memStore{m: make(map[string]*model.DownloadObject)}

	task1Canonical := &model.DownloadObject{
		TaskID: "t1",
		URL:    "u1",
		Metadata: map[string]string{
			"title":         "【高画质】CLUB-100",
			"task_type":     task.TypeTktube,
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusCompleted,
	}
	task1Pending := &model.DownloadObject{
		TaskID: "t1",
		URL:    "u2",
		Metadata: map[string]string{
			"title":         "CLUB-100C",
			"task_type":     task.TypeTktube,
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusPending,
	}
	task2Pending := &model.DownloadObject{
		TaskID: "t2",
		URL:    "u3",
		Metadata: map[string]string{
			"title":         "CLUB-100C",
			"task_type":     task.TypeTktube,
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusPending,
	}
	missingTaskID := &model.DownloadObject{
		TaskID: "",
		URL:    "u4",
		Metadata: map[string]string{
			"title":         "CLUB-100C",
			"task_type":     task.TypeTktube,
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusPending,
	}
	_ = store.Update(task1Canonical)
	_ = store.Update(task1Pending)
	_ = store.Update(task2Pending)
	_ = store.Update(missingTaskID)

	task1 := &fakeTktTask{id: "t1", st: store, objs: []*model.DownloadObject{task1Canonical, task1Pending}}

	m.applyGroupPriorityPolicies(task1, task1Canonical)

	if task1Pending.Status != dlcore.StatusCancelled {
		t.Fatalf("expect task1 pending object cancelled, got %s", task1Pending.Status)
	}
	if task1Pending.Extra["redirect_url"] != task1Canonical.URL {
		t.Fatalf("expect task1 redirect_url=%s, got %v", task1Canonical.URL, task1Pending.Extra["redirect_url"])
	}
	if task2Pending.Status != dlcore.StatusPending {
		t.Fatalf("expect task2 pending object untouched, got %s", task2Pending.Status)
	}
	if _, ok := task2Pending.Extra["redirect_url"]; ok {
		t.Fatalf("expect task2 redirect_url unset, got %v", task2Pending.Extra["redirect_url"])
	}
	if missingTaskID.Status != dlcore.StatusPending {
		t.Fatalf("expect object with empty TaskID ignored, got %s", missingTaskID.Status)
	}
	if _, ok := missingTaskID.Extra["redirect_url"]; ok {
		t.Fatalf("expect empty-TaskID redirect_url unset, got %v", missingTaskID.Extra["redirect_url"])
	}
}

func TestApplyGroupPriorityPolicies_IgnoresSameTaskIDWithDifferentTaskType(t *testing.T) {
	m := &Manager{}
	store := &memStore{m: make(map[string]*model.DownloadObject)}

	canonical := &model.DownloadObject{
		TaskID: "shared-task",
		URL:    "u1",
		Metadata: map[string]string{
			"title":         "【高画质】CLUB-100",
			"task_type":     task.TypeTktube,
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusCompleted,
	}
	sameTaskIDWrongType := &model.DownloadObject{
		TaskID: "shared-task",
		URL:    "u2",
		Metadata: map[string]string{
			"title":         "CLUB-100C",
			"task_type":     task.TypeHanime,
			"content_group": "CLUB-100",
		},
		Extra:  map[string]any{},
		Status: dlcore.StatusPending,
	}
	_ = store.Update(canonical)
	_ = store.Update(sameTaskIDWrongType)

	task1 := &fakeTktTask{
		id:   "shared-task",
		typ:  task.TypeTktube,
		st:   store,
		objs: []*model.DownloadObject{canonical, sameTaskIDWrongType},
	}

	m.applyGroupPriorityPolicies(task1, canonical)

	if sameTaskIDWrongType.Status != dlcore.StatusPending {
		t.Fatalf("expect different task_type object untouched, got %s", sameTaskIDWrongType.Status)
	}
	if _, ok := sameTaskIDWrongType.Extra["redirect_url"]; ok {
		t.Fatalf("expect different task_type redirect_url unset, got %v", sameTaskIDWrongType.Extra["redirect_url"])
	}
}
