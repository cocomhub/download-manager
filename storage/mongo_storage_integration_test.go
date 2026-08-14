// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package storage

import (
	"os"
	"strconv"
	"testing"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMongoStorage_CRUD(t *testing.T) {
	ctx := t.Context()

	// Start MongoDB container
	mongoContainer, err := mongodb.Run(ctx, "mongo:8")
	if err != nil {
		t.Fatalf("failed to start mongo container: %v", err)
	}
	defer func() {
		if err := mongoContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate mongo container: %v", err)
		}
	}()

	connStr, err := mongoContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	// Initialize mongo client with the container URI
	err = InitMongoClients([]struct{ Name, URI string }{
		{Name: "test", URI: connStr},
	})
	if err != nil {
		t.Fatalf("failed to init mongo clients: %v", err)
	}
	defer CloseAllMongoClients()

	// Create storage instance
	st, err := NewMongoStorage(map[string]string{
		"source":     "test",
		"database":   "testdb",
		"collection": "objects",
	})
	if err != nil {
		t.Fatalf("failed to create mongo storage: %v", err)
	}

	// Test Create (via Update with upsert)
	obj := &model.DownloadObject{
		TaskID:   "task1",
		URL:      "http://example.com/file1",
		SavePath: "/downloads/file1",
		Status:   "pending",
		Progress: 0,
		Metadata: map[string]string{"author": "test"},
		Extra:    map[string]any{"tags": []string{"a", "b"}},
	}
	if err := st.Update(obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	// Test Get
	got, err := st.Get("http://example.com/file1")
	if err != nil {
		t.Fatalf("failed to get object: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil object")
	}
	if got.URL != "http://example.com/file1" {
		t.Errorf("URL mismatch: got %q, want %q", got.URL, "http://example.com/file1")
	}
	if got.Status != "pending" {
		t.Errorf("Status mismatch: got %q, want %q", got.Status, "pending")
	}
	if got.Metadata["author"] != "test" {
		t.Errorf("Metadata mismatch: got %v", got.Metadata)
	}

	// Test Get with non-existent key
	notFound, err := st.Get("http://example.com/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error for non-existent: %v", err)
	}
	if notFound != nil {
		t.Fatal("expected nil for non-existent object")
	}

	// Test Update
	obj.Status = "completed"
	obj.Progress = 100
	if err := st.Update(obj); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}
	updated, err := st.Get("http://example.com/file1")
	if err != nil {
		t.Fatalf("failed to get updated object: %v", err)
	}
	if updated.Status != "completed" || updated.Progress != 100 {
		t.Errorf("update failed: status=%s progress=%d", updated.Status, updated.Progress)
	}

	// Test Exists
	exists, err := st.Exists([]string{"http://example.com/file1", "http://example.com/nonexistent"})
	if err != nil {
		t.Fatalf("failed to check exists: %v", err)
	}
	if !exists["http://example.com/file1"] {
		t.Error("expected file1 to exist")
	}
	if exists["http://example.com/nonexistent"] {
		t.Error("expected nonexistent to not exist")
	}

	// Test Search
	results, err := st.Search(&core.StorageQuery{
		Filter: core.StorageFilter{
			TaskIDs:  []string{"task1"},
			Statuses: []string{"completed"},
		},
	})
	if err != nil {
		t.Fatalf("failed to search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].URL != "http://example.com/file1" {
		t.Errorf("search result URL mismatch: %s", results[0].URL)
	}

	// Test Search with metadata filter
	results, err = st.Search(&core.StorageQuery{
		Filter: core.StorageFilter{
			Metadata: map[string]string{"author": "test"},
		},
	})
	if err != nil {
		t.Fatalf("failed to search by metadata: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result from metadata search, got %d", len(results))
	}

	// Test Count
	count, err := st.Count(&core.StorageQuery{
		Filter: core.StorageFilter{
			TaskIDs: []string{"task1"},
		},
	})
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}

	// Test Delete
	if err := st.Delete("http://example.com/file1"); err != nil {
		t.Fatalf("failed to delete: %v", err)
	}
	deleted, err := st.Get("http://example.com/file1")
	if err != nil {
		t.Fatalf("failed to get after delete: %v", err)
	}
	if deleted != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestMongoStorage_SearchPagination(t *testing.T) {
	ctx := t.Context()

	mongoContainer, err := mongodb.Run(ctx, "mongo:8")
	if err != nil {
		t.Fatalf("failed to start mongo container: %v", err)
	}
	defer func() {
		if err := mongoContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate mongo container: %v", err)
		}
	}()

	connStr, err := mongoContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	err = InitMongoClients([]struct{ Name, URI string }{
		{Name: "test", URI: connStr},
	})
	if err != nil {
		t.Fatalf("failed to init mongo clients: %v", err)
	}
	defer CloseAllMongoClients()

	st, err := NewMongoStorage(map[string]string{
		"source":     "test",
		"database":   "testdb",
		"collection": "pagination_test",
	})
	if err != nil {
		t.Fatalf("failed to create mongo storage: %v", err)
	}

	// Insert 5 objects
	for i := 1; i <= 5; i++ {
		obj := &model.DownloadObject{
			TaskID:   "task_paginate",
			URL:      "http://example.com/file" + strconv.Itoa(i),
			SavePath: "/downloads/file" + strconv.Itoa(i),
			Status:   "pending",
			Metadata: map[string]string{"index": strconv.Itoa(i)},
		}
		if err := st.Update(obj); err != nil {
			t.Fatalf("failed to insert object %d: %v", i, err)
		}
	}

	// Test limit
	results, err := st.Search(&core.StorageQuery{
		Filter: core.StorageFilter{TaskIDs: []string{"task_paginate"}},
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("failed to search with limit: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results with limit=2, got %d", len(results))
	}

	// Test offset
	results, err = st.Search(&core.StorageQuery{
		Filter: core.StorageFilter{TaskIDs: []string{"task_paginate"}},
		Limit:  2,
		Offset: 2,
	})
	if err != nil {
		t.Fatalf("failed to search with offset: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results with offset=2, got %d", len(results))
	}

	// Test search by text
	results, err = st.Search(&core.StorageQuery{
		Filter: core.StorageFilter{
			TaskIDs: []string{"task_paginate"},
			Search:  "file3",
		},
	})
	if err != nil {
		t.Fatalf("failed to search by text: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for search 'file3', got %d", len(results))
	}
	if results[0].URL != "http://example.com/file3" {
		t.Errorf("search result mismatch: %s", results[0].URL)
	}
}

func TestMongoStorage_ContentGroupRepresentatives(t *testing.T) {
	ctx := t.Context()

	mongoContainer, err := mongodb.Run(ctx, "mongo:8")
	if err != nil {
		t.Fatalf("failed to start mongo container: %v", err)
	}
	defer func() {
		if err := mongoContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate mongo container: %v", err)
		}
	}()

	connStr, err := mongoContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	err = InitMongoClients([]struct{ Name, URI string }{
		{Name: "test", URI: connStr},
	})
	if err != nil {
		t.Fatalf("failed to init mongo clients: %v", err)
	}
	defer CloseAllMongoClients()

	st, err := NewMongoStorage(map[string]string{
		"source":     "test",
		"database":   "testdb",
		"collection": "group_rep_test",
	})
	if err != nil {
		t.Fatalf("failed to create mongo storage: %v", err)
	}

	seed := func(taskID, url, group, date string) {
		obj := &model.DownloadObject{
			TaskID:   taskID,
			URL:      url,
			SavePath: "/downloads/" + url,
			Status:   "completed",
			Metadata: map[string]string{
				"title":         url,
				"content_group": group,
				"date":          date,
				"task_type":     "mxs",
			},
			Extra: map[string]any{
				"files": []map[string]string{{"url": url + "/1.jpg", "path": "/x/1.jpg"}},
			},
		}
		if err := st.Update(obj); err != nil {
			t.Fatalf("seed %s: %v", url, err)
		}
	}

	// 组 1094：3 章，date 零填充章节号，最新 "0000000003"
	seed("mxs-demo", "c1", "1094", "0000000001")
	seed("mxs-demo", "c2", "1094", "0000000002")
	seed("mxs-demo", "c3", "1094", "0000000003")
	// 组 1095：2 章
	seed("mxs-demo", "c4", "1095", "0000000010")
	seed("mxs-demo", "c5", "1095", "0000000020")
	// 空 content_group / 缺失 content_group → 不应计入 total
	seed("mxs-demo", "c6", "", "0000000100")
	// 其他任务的对象不应计入（按 task_id 过滤）
	other := &model.DownloadObject{
		TaskID:   "other-task",
		URL:      "c7",
		SavePath: "/downloads/c7",
		Status:   "pending",
		Metadata: map[string]string{"title": "c7", "content_group": "1094", "date": "0000000099"},
	}
	if err := st.Update(other); err != nil {
		t.Fatalf("seed c7: %v", err)
	}

	// 全量（不分页）
	objs, total, err := st.ContentGroupRepresentatives("mxs-demo", "", "", 1, -1)
	if err != nil {
		t.Fatalf("ContentGroupRepresentatives: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2 (空组/缺失组/其他任务不计)", total)
	}
	if len(objs) != 2 {
		t.Fatalf("reps len = %d, want 2", len(objs))
	}
	// 组间按代表 date 降序：1095(0000000020) 在前，1094(0000000003) 在后
	if objs[0].Metadata["content_group"] != "1095" || objs[0].Metadata["date"] != "0000000020" {
		t.Errorf("objs[0] = %v", objs[0].Metadata)
	}
	if objs[0].GetGroupSize() != 2 {
		t.Errorf("objs[0] group_size = %d, want 2", objs[0].GetGroupSize())
	}
	if objs[1].Metadata["content_group"] != "1094" || objs[1].Metadata["date"] != "0000000003" {
		t.Errorf("objs[1] = %v", objs[1].Metadata)
	}
	if objs[1].GetGroupSize() != 3 {
		t.Errorf("objs[1] group_size = %d, want 3", objs[1].GetGroupSize())
	}
	// 大数组字段应在投影中被剔除
	if _, ok := objs[0].Extra["files"]; ok {
		t.Errorf("extra.files should be projected out, got %v", objs[0].Extra["files"])
	}

	// 分页：page 1, limit 1 → 只返回 1095，total 仍为 2
	paged, total2, err := st.ContentGroupRepresentatives("mxs-demo", "", "", 1, 1)
	if err != nil {
		t.Fatalf("ContentGroupRepresentatives paged: %v", err)
	}
	if total2 != 2 || len(paged) != 1 {
		t.Errorf("paged: total=%d len=%d, want 2/1", total2, len(paged))
	}
	if len(paged) == 1 && paged[0].Metadata["content_group"] != "1095" {
		t.Errorf("paged[0] = %v", paged[0].Metadata)
	}

	// search 过滤：匹配 url/title/tags 含 "c3" 的对象（组 1094 内 date 最大者为 c3）。
	searched, totalS, err := st.ContentGroupRepresentatives("mxs-demo", "c3", "", 1, -1)
	if err != nil {
		t.Fatalf("ContentGroupRepresentatives search: %v", err)
	}
	if totalS != 1 || len(searched) != 1 {
		t.Errorf("search: total=%d len=%d, want 1/1", totalS, len(searched))
	}
	if len(searched) == 1 && searched[0].Metadata["date"] != "0000000003" {
		t.Errorf("search rep date = %q, want newest c3 0000000003", searched[0].Metadata["date"])
	}

	// status 过滤：不存在的状态 → 空结果，total 0。
	stEmpty, totalSt, err := st.ContentGroupRepresentatives("mxs-demo", "", "nonexistent", 1, -1)
	if err != nil {
		t.Fatalf("ContentGroupRepresentatives status: %v", err)
	}
	if totalSt != 0 || len(stEmpty) != 0 || stEmpty == nil {
		t.Errorf("status filter: total=%d len=%d nil=%v, want 0/0/false", totalSt, len(stEmpty), stEmpty == nil)
	}

	// page 越界 → 空但非 nil
	empty, total3, err := st.ContentGroupRepresentatives("mxs-demo", "", "", 99, 1)
	if err != nil {
		t.Fatalf("ContentGroupRepresentatives out-of-range: %v", err)
	}
	if total3 != 2 || len(empty) != 0 || empty == nil {
		t.Errorf("out-of-range: total=%d len=%d nil=%v, want 2/0/false", total3, len(empty), empty == nil)
	}
}

func TestMongoStorage_IndexesCreated(t *testing.T) {
	ctx := t.Context()

	mongoContainer, err := mongodb.Run(ctx, "mongo:8")
	if err != nil {
		t.Fatalf("failed to start mongo container: %v", err)
	}
	defer func() {
		if err := mongoContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate mongo container: %v", err)
		}
	}()

	connStr, err := mongoContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	err = InitMongoClients([]struct{ Name, URI string }{
		{Name: "test", URI: connStr},
	})
	if err != nil {
		t.Fatalf("failed to init mongo clients: %v", err)
	}
	defer CloseAllMongoClients()

	st, err := NewMongoStorage(map[string]string{
		"source":     "test",
		"database":   "testdb",
		"collection": "index_test",
	})
	if err != nil {
		t.Fatalf("failed to create mongo storage: %v", err)
	}

	// Access the collection's indexes
	cursor, err := st.collection.Indexes().List(ctx)
	if err != nil {
		t.Fatalf("failed to list indexes: %v", err)
	}
	defer cursor.Close(ctx)

	var indexNames []string
	for cursor.Next(ctx) {
		var idx bson.M
		if err := cursor.Decode(&idx); err != nil {
			t.Fatalf("failed to decode index: %v", err)
		}
		indexNames = append(indexNames, idx["name"].(string))
	}

	// Check expected indexes
	expectedIndexes := []string{"_id_", "url_unique", "id_unique", "task_status", "task_group", "task_date_desc", "title_lookup", "collection_order"}
	for _, expected := range expectedIndexes {
		found := false
		for _, name := range indexNames {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected index %q not found in %v", expected, indexNames)
		}
	}
}

func TestMain(m *testing.M) {
	// Ensure no leftover mongo clients from previous tests
	CloseAllMongoClients()
	os.Exit(m.Run())
}
