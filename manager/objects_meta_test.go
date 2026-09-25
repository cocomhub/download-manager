// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"log/slog"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// metaStore 轻量查询测试存储（记录 Light 标志）。
type metaStore struct {
	objs      []*model.DownloadObject
	lightUsed bool
}

func (s *metaStore) Get(id string) (*model.DownloadObject, error) { return nil, nil }
func (s *metaStore) Update(obj *model.DownloadObject) error       { return nil }
func (s *metaStore) Delete(id string) error                       { return nil }
func (s *metaStore) Search(query *core.StorageQuery) ([]*model.DownloadObject, error) {
	s.lightUsed = query.Light
	// content_group 过滤
	if cg := query.Filter.Metadata["content_group"]; cg != "" {
		var out []*model.DownloadObject
		for _, o := range s.objs {
			o.RLock()
			g := o.Metadata["content_group"]
			o.RUnlock()
			if g == cg {
				out = append(out, o)
			}
		}
		return out, nil
	}
	return s.objs, nil
}
func (s *metaStore) Count(query *core.StorageQuery) (int64, error) { return int64(len(s.objs)), nil }
func (s *metaStore) Exists(ids []string) (map[string]bool, error)  { return nil, nil }

// metaTask 轻量查询测试任务。
type metaTask struct {
	id string
	st core.Storage
}

func (t *metaTask) ID() string                           { return t.id }
func (t *metaTask) Type() string                         { return "meta" }
func (t *metaTask) Logger() *slog.Logger                 { return slog.Default() }
func (t *metaTask) Storage() core.Storage                { return t.st }
func (t *metaTask) SetDownloader(dl core.Downloader)     {}
func (t *metaTask) GetDownloadHeaders() map[string]string { return map[string]string{} }
func (t *metaTask) GetDownloadObjects() ([]*model.DownloadObject, error) { return nil, nil }
func (t *metaTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error { return nil }
func (t *metaTask) ResolveObject(_ context.Context, _ *model.DownloadObject) error { return nil }
func (t *metaTask) Close() error                                    { return nil }
func (t *metaTask) GetAllObjects(lock bool) []*model.DownloadObject { return nil }
func (t *metaTask) Start() error                                    { return nil }
func (t *metaTask) Concurrency() int                                { return 1 }
func (t *metaTask) SetConcurrency(c int) error                      { return nil }
func (t *metaTask) RefreshInterval() int                            { return 0 }
func (t *metaTask) SetRefreshInterval(i int) error                  { return nil }

// TestGetTaskObjectsMeta_LightFilter 验证轻量查询使用 Light 投影 + content_group 过滤。
func TestGetTaskObjectsMeta_LightFilter(t *testing.T) {
	cfg := &config.Config{Tasks: []config.Task{{ID: "t1", Type: "meta"}}}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)
	store := &metaStore{objs: []*model.DownloadObject{
		{TaskID: "t1", URL: "u1", Metadata: map[string]string{"content_group": "g1", "title": "book1"}, Status: model.StatusCompleted},
		{TaskID: "t1", URL: "u2", Metadata: map[string]string{"content_group": "g2", "title": "book2"}, Status: model.StatusCompleted},
	}}
	m.tasks.Store("t1", &metaTask{id: "t1", st: store})

	objs, err := m.GetTaskObjectsMeta("t1", "g1", "", "", 0)
	if err != nil {
		t.Fatalf("GetTaskObjectsMeta: %v", err)
	}
	if len(objs) != 1 || objs[0].URL != "u1" {
		t.Fatalf("got %d objs, want 1 (content_group=g1)", len(objs))
	}
	if !store.lightUsed {
		t.Fatal("Light projection should be used")
	}
}
