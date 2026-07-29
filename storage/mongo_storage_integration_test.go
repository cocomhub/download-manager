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
