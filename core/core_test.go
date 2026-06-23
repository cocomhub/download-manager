// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/cocomhub/download-manager/model"
)

// TestStorageFilter 楠岃瘉 StorageFilter 缁撴瀯浣撳瓧娈?func TestStorageFilter(t *testing.T) {
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

// TestStorageQuery 楠岃瘉 StorageQuery 鏋勫缓鍣?func TestStorageQuery(t *testing.T) {
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

// TestStorageSort 楠岃瘉鎺掑簭瀹氫箟
func TestStorageSort(t *testing.T) {
	s := StorageSort{Field: "progress", Desc: true}
	if s.Field != "progress" {
		t.Errorf("Field = %q, want progress", s.Field)
	}
	if !s.Desc {
		t.Error("Desc = false, want true")
	}
}

// TestEventTypeConstants 楠岃瘉浜嬩欢绫诲瀷甯搁噺
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

// TestEventStruct 楠岃瘉浜嬩欢缁撴瀯浣?func TestEventStruct(t *testing.T) {
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

// TestDownloaderInterface 楠岃瘉 Downloader 鎺ュ彛瀹氫箟浜?Name 鏂规硶
func TestDownloaderInterface(t *testing.T) {
	// 缂栬瘧鏃舵鏌ワ細纭繚 Downloader 鎺ュ彛瀛樺湪骞跺寘鍚?Name 鏂规硶
	var _ Downloader = &mockDownloader{}
}

// mockDownloader 瀹炵幇 Downloader 鎺ュ彛鐢ㄤ簬缂栬瘧妫€鏌?type mockDownloader struct{}

func (m *mockDownloader) Download(_ *model.DownloadObject, _ map[string]string) error {
	return nil
}

func (m *mockDownloader) Name() string {
	return "mock"
}
