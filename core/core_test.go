// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/cocomhub/download-manager/model"
)

// TestStorageFilter 验证 StorageFilter 结构体字段
func TestStorageFilter(t *testing.T) {
	f := StorageFilter{
		TaskIDs:  []string{"task1"},
		Statuses: []string{"pending", "downloading"},
		Search:   "keyword",
	}
	if len(f.TaskIDs) != 1 || f.TaskIDs[0] != "task1" {
		t.Errorf("TaskIDs = %v, want [task1]", f.TaskIDs)
	}
	if len(f.Statuses) != 2 {
		t.Errorf("Statuses length = %d, want 2", len(f.Statuses))
	}
	if f.Search != "keyword" {
		t.Errorf("Search = %q, want keyword", f.Search)
	}
}

// TestStorageQuery 验证 StorageQuery 构建器
func TestStorageQuery(t *testing.T) {
	q := &StorageQuery{
		Filter: StorageFilter{
			TaskIDs:  []string{"task1"},
			Statuses: []string{"pending"},
		},
		Sort:   []StorageSort{{Field: "created_at", Desc: true}},
		Offset: 0,
		Limit:  10,
	}
	if len(q.Filter.TaskIDs) != 1 || q.Filter.TaskIDs[0] != "task1" {
		t.Errorf("Filter.TaskIDs = %v, want [task1]", q.Filter.TaskIDs)
	}
	if q.Offset != 0 || q.Limit != 10 {
		t.Errorf("Offset/Limit = %d/%d, want 0/10", q.Offset, q.Limit)
	}
	if len(q.Sort) != 1 || q.Sort[0].Field != "created_at" || !q.Sort[0].Desc {
		t.Errorf("Sort = %v, want [{created_at true}]", q.Sort)
	}
}

// TestStorageSort 验证排序定义
func TestStorageSort(t *testing.T) {
	s := StorageSort{Field: "progress", Desc: true}
	if s.Field != "progress" {
		t.Errorf("Field = %q, want progress", s.Field)
	}
	if !s.Desc {
		t.Error("Desc = false, want true")
	}
}

// TestEventTypeConstants 验证事件类型常量
func TestEventTypeConstants(t *testing.T) {
	tests := []struct {
		name  string
		value EventType
		want  string
	}{
		{"EventTaskUpdate", EventTaskUpdate, "task_update"},
		{"EventTaskListChange", EventTaskListChange, "task_list_change"},
		{"EventObjectUpdate", EventObjectUpdate, "object_update"},
		{"EventSharedObjectUpdate", EventSharedObjectUpdate, "shared_object_update"},
		{"EventProgressBatch", EventProgressBatch, "progress_batch"},
	}
	for _, tt := range tests {
		if string(tt.value) != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, string(tt.value), tt.want)
		}
	}
}

// TestEventStruct 验证事件结构体
func TestEventStruct(t *testing.T) {
	e := Event{
		Type:    EventObjectUpdate,
		Payload: &model.DownloadObject{URL: "http://example.com/file.zip"},
	}
	if e.Type != EventObjectUpdate {
		t.Errorf("Type = %q, want %q", e.Type, EventObjectUpdate)
	}
	if e.Payload == nil {
		t.Fatal("Payload is nil")
	}
	obj, ok := e.Payload.(*model.DownloadObject)
	if !ok {
		t.Fatal("Payload is not *model.DownloadObject")
	}
	if obj.URL != "http://example.com/file.zip" {
		t.Errorf("Payload URL = %q, want http://example.com/file.zip", obj.URL)
	}
}

// TestDownloaderInterface 验证 Downloader 接口定义了 Name 方法
func TestDownloaderInterface(t *testing.T) {
	// 编译时检查：确保 Downloader 接口存在并包含 Name 方法
	var _ Downloader = &mockDownloader{}
}

// mockDownloader 实现 Downloader 接口用于编译检查
type mockDownloader struct{}

func (m *mockDownloader) Download(_ *model.DownloadObject, _ map[string]string) error {
	return nil
}

func (m *mockDownloader) Name() string {
	return "mock"
}
