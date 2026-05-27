// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"log/slog"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

type mockTask struct {
	id   string
	typ  string
	objs []*model.DownloadObject
}

func (m *mockTask) ID() string                            { return m.id }
func (m *mockTask) Type() string                          { return m.typ }
func (m *mockTask) Logger() *slog.Logger                  { return slog.Default() }
func (m *mockTask) Storage() core.Storage                 { return nil }
func (m *mockTask) SetDownloader(core.Downloader)         {}
func (m *mockTask) GetDownloadHeaders() map[string]string { return map[string]string{} }
func (m *mockTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	return []*model.DownloadObject{}, nil
}
func (m *mockTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	return nil
}
func (m *mockTask) Concurrency() int                                { return 1 }
func (m *mockTask) SetConcurrency(int) error                        { return nil }
func (m *mockTask) RefreshInterval() int                            { return 0 }
func (m *mockTask) SetRefreshInterval(int) error                    { return nil }
func (m *mockTask) Start() error                                    { return nil }
func (m *mockTask) Close() error                                    { return nil }
func (m *mockTask) GetAllObjects(lock bool) []*model.DownloadObject { return m.objs }

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
	objs2 := res2["objects"].([]*model.DownloadObject)
	if len(objs2) != 1 || objs2[0].TaskID != "t2" {
		t.Fatalf("expect only t2 matched, got: %+v", objs2)
	}

	res3, err := m.AggregateObjects(1, -1, "", "", "", []string{})
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}
	objs3 := res3["objects"].([]*model.DownloadObject)
	if len(objs3) != 2 {
		t.Fatalf("expect 2 objects when no type filter, got: %d", len(objs3))
	}
}
