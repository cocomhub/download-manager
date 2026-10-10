// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package storage

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// mongoTestURI 返回集成测试用的 mongo URI：优先复用外部实例（MONGO_TEST_URI ——
// CI 的 services 容器或 scripts/test-mongo.sh 起的容器），否则用 testcontainers 起容器；
// 两者都不可用时 t.Skip（不失败，对齐 sproxy 的 vault 集成测试约定）。
func mongoTestURI(t *testing.T) string {
	t.Helper()
	if uri := os.Getenv("MONGO_TEST_URI"); uri != "" {
		return uri
	}
	ctx := t.Context()
	c, err := mongodb.Run(ctx, "mongo:8")
	if err != nil {
		t.Skipf("mongo 不可用（未设 MONGO_TEST_URI 且 testcontainers 启动失败）: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })
	uri, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}
	return uri
}

// mongoTestStorage 起/复用 mongo 并返回指向独立数据库的存储实例
// （独立库名避免复用外部实例时用例互相污染）。
func mongoTestStorage(t *testing.T, collection string) *MongoStorage {
	t.Helper()
	uri := mongoTestURI(t)
	if err := InitMongoClients([]struct{ Name, URI string }{{Name: "test", URI: uri}}); err != nil {
		t.Fatalf("failed to init mongo clients: %v", err)
	}
	t.Cleanup(CloseAllMongoClients)
	st, err := NewMongoStorage(map[string]string{
		"source":     "test",
		"database":   fmt.Sprintf("dm_it_%d", time.Now().UnixNano()),
		"collection": collection,
	})
	if err != nil {
		t.Fatalf("failed to create mongo storage: %v", err)
	}
	// 复用外部容器时清理本用例的库，避免累积 dm_it_*
	t.Cleanup(func() {
		_ = st.collection.Database().Drop(context.Background())
	})
	return st
}

func TestMongoStorage_CRUD(t *testing.T) {
	st := mongoTestStorage(t, "objects")

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
	st := mongoTestStorage(t, "pagination_test")

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

func TestMongoStorage_IndexesCreated(t *testing.T) {
	ctx := t.Context()

	st := mongoTestStorage(t, "index_test")

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
	expectedIndexes := []string{"_id_", "url_unique", "id_unique", "task_status", "task_group", "task_date_desc", "title_lookup", "collection_order", "task_version"}
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

// TestMongoStorage_CloudDownloadRoundTrip 验证「云端下载」开关在真实 mongo 上能开/关并落库
// （曾两次静默失效：Snapshot 漏拷 → 开启不落库；bson omitempty → 关闭不落库）。
func TestMongoStorage_CloudDownloadRoundTrip(t *testing.T) {
	st := mongoTestStorage(t, "cloud_download")
	const u = "http://example.com/cd"

	obj := &model.DownloadObject{TaskID: "t-cd", URL: u, SavePath: "/tmp/cd", Status: "pending"}
	if err := st.Update(obj); err != nil {
		t.Fatalf("update(init): %v", err)
	}
	if got, err := st.Get(u); err != nil || got == nil || got.IsCloudDownload() {
		t.Fatalf("初始应为 false, got %v err=%v", got, err)
	}

	obj.SetCloudDownload(true)
	if err := st.Update(obj); err != nil {
		t.Fatalf("update(true): %v", err)
	}
	if got, err := st.Get(u); err != nil || got == nil || !got.IsCloudDownload() {
		t.Fatalf("开启后 mongo 未落库（Snapshot 漏拷？）: %v err=%v", got, err)
	}

	obj.SetCloudDownload(false)
	if err := st.Update(obj); err != nil {
		t.Fatalf("update(false): %v", err)
	}
	if got, err := st.Get(u); err != nil || got == nil || got.IsCloudDownload() {
		t.Fatalf("关闭后 mongo 未更新（bson omitempty？）: %v err=%v", got, err)
	}
}

// TestMongoStorage_VersionLTMatchesMissingField 验证 version 字段缺失的旧文档也能被版本
// 升级扫描命中（mongo 上 version=0 不出现在文档里，{$lt:N} 不匹配缺字段）。
func TestMongoStorage_VersionLTMatchesMissingField(t *testing.T) {
	ctx := t.Context()
	st := mongoTestStorage(t, "version_lt")

	if _, err := st.collection.InsertOne(ctx, bson.M{
		"task_id": "t-old", "url": "http://example.com/old", "save_path": "/tmp/old",
		"status": "pending", "progress": 0,
	}); err != nil {
		t.Fatalf("insert legacy doc: %v", err)
	}
	res, err := st.Search(&core.StorageQuery{
		Filter: core.StorageFilter{TaskIDs: []string{"t-old"}, VersionLT: 5},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("缺 version 的旧文档应被 VersionLT 命中, got %d", len(res))
	}
}

// TestMongoStorage_UpdateFieldsPartial 验证局部更新只改指定字段、不覆盖其它字段
// （消除整文档 $set 的读-改-写窗口：并发下载器写入的 status/progress 不会被陈旧快照覆盖）。
func TestMongoStorage_UpdateFieldsPartial(t *testing.T) {
	st := mongoTestStorage(t, "update_fields")
	const u = "http://example.com/uf"

	obj := &model.DownloadObject{TaskID: "t-uf", URL: u, SavePath: "/tmp/uf", Status: "downloading", Progress: 42}
	if err := st.Update(obj); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := st.UpdateFields(u, map[string]any{"cloud_download": true}); err != nil {
		t.Fatalf("UpdateFields: %v", err)
	}
	got, err := st.Get(u)
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if !got.IsCloudDownload() {
		t.Fatal("局部更新未生效")
	}
	if got.GetStatus() != "downloading" || got.GetProgress() != 42 {
		t.Fatalf("局部更新覆盖了其它字段: status=%s progress=%d", got.GetStatus(), got.GetProgress())
	}
}

// TestMongoStorage_StatusPartialUpdateKeepsOption 验证「状态局部更新」不会回退 per-object 选项
// （根治：BaseTask.persistObject 对实现 ObjectFieldUpdater 的存储改走局部更新，
// 陈旧运行时副本不再整文档覆盖 cloud_download）。
func TestMongoStorage_StatusPartialUpdateKeepsOption(t *testing.T) {
	st := mongoTestStorage(t, "stale_opt")
	const u = "http://example.com/stale"

	obj := &model.DownloadObject{TaskID: "t-stale", URL: u, SavePath: "/tmp/stale", Status: "pending"}
	if err := st.Update(obj); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := st.UpdateFields(u, map[string]any{"cloud_download": true}); err != nil {
		t.Fatalf("UpdateFields(cloud_download): %v", err)
	}

	// 模拟陈旧运行时副本的状态写入（只写状态字段）
	if err := st.UpdateFields(u, map[string]any{"status": "downloading", "progress": 7}); err != nil {
		t.Fatalf("UpdateFields(status): %v", err)
	}
	got, err := st.Get(u)
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if !got.IsCloudDownload() {
		t.Fatal("状态局部更新回退了 per-object 选项")
	}
	if got.GetStatus() != "downloading" || got.GetProgress() != 7 {
		t.Fatalf("状态未写入: status=%s progress=%d", got.GetStatus(), got.GetProgress())
	}

	// 白名单校验：非白名单字段必须被拒绝
	if err := st.UpdateFields(u, map[string]any{"not_allowed": 1}); err == nil {
		t.Fatal("非白名单字段应被拒绝")
	}
}

func TestMain(m *testing.M) {
	// Ensure no leftover mongo clients from previous tests
	CloseAllMongoClients()
	os.Exit(m.Run())
}
