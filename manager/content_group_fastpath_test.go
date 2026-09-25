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

// fastPathStore 模拟存储实现 ContentGroupRepresentatives（快路径）。
type fastPathStore struct {
	objs      []*model.DownloadObject
	callCount int
}

func (s *fastPathStore) Get(id string) (*model.DownloadObject, error) { return nil, nil }
func (s *fastPathStore) Update(obj *model.DownloadObject) error       { return nil }
func (s *fastPathStore) Delete(id string) error                       { return nil }
func (s *fastPathStore) Search(query *core.StorageQuery) ([]*model.DownloadObject, error) {
	return s.objs, nil
}
func (s *fastPathStore) Count(query *core.StorageQuery) (int64, error) { return int64(len(s.objs)), nil }
func (s *fastPathStore) Exists(ids []string) (map[string]bool, error)  { return nil, nil }

// ContentGroupRepresentatives 快路径实现：记录调用 + 返回 max-date 代表。
func (s *fastPathStore) ContentGroupRepresentatives(taskID, search, status string, page, limit int64) ([]*model.DownloadObject, int64, error) {
	s.callCount++
	// 模拟分组代表：每个 content_group 返回一个代表
	seen := make(map[string]bool)
	var reps []*model.DownloadObject
	for _, o := range s.objs {
		o.RLock()
		g := o.Metadata["content_group"]
		o.RUnlock()
		if g == "" || seen[g] {
			continue
		}
		seen[g] = true
		reps = append(reps, o)
	}
	return reps, int64(len(reps)), nil
}

// fastPathTask 模拟无自定义代表语义的任务（未实现 ContentGroupProvider）。
type fastPathTask struct {
	id string
	st core.Storage
}

func (f *fastPathTask) ID() string                          { return f.id }
func (f *fastPathTask) Type() string                        { return "fastpath" }
func (f *fastPathTask) Logger() *slog.Logger                { return slog.Default() }
func (f *fastPathTask) Storage() core.Storage               { return f.st }
func (f *fastPathTask) SetDownloader(dl core.Downloader)    {}
func (f *fastPathTask) GetDownloadHeaders() map[string]string { return map[string]string{} }
func (f *fastPathTask) GetDownloadObjects() ([]*model.DownloadObject, error) { return nil, nil }
func (f *fastPathTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error { return nil }
func (f *fastPathTask) ResolveObject(_ context.Context, _ *model.DownloadObject) error { return nil }
func (f *fastPathTask) Close() error                                    { return nil }
func (f *fastPathTask) GetAllObjects(lock bool) []*model.DownloadObject { return nil }
func (f *fastPathTask) Start() error                                    { return nil }
func (f *fastPathTask) Concurrency() int                                { return 1 }
func (f *fastPathTask) SetConcurrency(c int) error                      { return nil }
func (f *fastPathTask) RefreshInterval() int                            { return 0 }
func (f *fastPathTask) SetRefreshInterval(i int) error                  { return nil }

// TestAggregateByContent_FastPath 验证单任务 + 快路径存储时走 mongo 聚合下推。
func TestAggregateByContent_FastPath(t *testing.T) {
	cfg := &config.Config{Tasks: []config.Task{{ID: "t1", Type: "fastpath"}}}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)

	store := &fastPathStore{objs: []*model.DownloadObject{
		{TaskID: "t1", URL: "u1", Metadata: map[string]string{"content_group": "g1", "date": "2026-01-01", "task_type": "fastpath"}, Status: model.StatusCompleted},
		{TaskID: "t1", URL: "u2", Metadata: map[string]string{"content_group": "g1", "date": "2026-02-01", "task_type": "fastpath"}, Status: model.StatusCompleted},
		{TaskID: "t1", URL: "u3", Metadata: map[string]string{"content_group": "g2", "date": "2026-03-01", "task_type": "fastpath"}, Status: model.StatusCompleted},
	}}
	m.tasks.Store("t1", &fastPathTask{id: "t1", st: store})

	res, err := m.AggregateByContent(1, 50, "", "", "", []string{"fastpath"})
	if err != nil {
		t.Fatalf("AggregateByContent: %v", err)
	}
	if store.callCount != 1 {
		t.Fatalf("fast path not used: callCount = %d, want 1", store.callCount)
	}
	objects := res["objects"].([]*model.DownloadObject)
	if len(objects) != 2 {
		t.Fatalf("got %d reps, want 2 (one per group)", len(objects))
	}
}

// TestAggregateByContent_NoFastPathWhenCustomPicker 任务实现 ContentGroupProvider
// （自定义代表语义）→ 不走快路径（走内存兜底）。
func TestAggregateByContent_NoFastPathWhenCustomPicker(t *testing.T) {
	cfg := &config.Config{Tasks: []config.Task{{ID: "t1", Type: "custom"}}}
	m := NewManager(cfg)
	m.cfgVal.Store(cfg)

	store := &fastPathStore{objs: []*model.DownloadObject{
		{TaskID: "t1", URL: "u1", Metadata: map[string]string{"content_group": "g1", "task_type": "custom"}, Status: model.StatusCompleted},
	}}
	// customTask 实现 ContentGroupProvider（自定义代表语义）
	m.tasks.Store("t1", &customPickerTask{fastPathTask: fastPathTask{id: "t1", st: store}})

	if _, err := m.AggregateByContent(1, 50, "", "", "", []string{"custom"}); err != nil {
		t.Fatalf("AggregateByContent: %v", err)
	}
	if store.callCount != 0 {
		t.Fatalf("fast path should NOT be used for custom picker task: callCount = %d", store.callCount)
	}
}

// customPickerTask 实现 ContentGroupProvider（自定义代表语义）。
type customPickerTask struct {
	fastPathTask
}

func (c *customPickerTask) ContentGroupKey(obj *model.DownloadObject) string {
	if obj == nil {
		return ""
	}
	return obj.Metadata["content_group"]
}
func (c *customPickerTask) VariantScore(obj *model.DownloadObject) int { return 1 }
func (c *customPickerTask) BackfillContentGroups() bool               { return false }
