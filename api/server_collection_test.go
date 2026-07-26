// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/manager"
	"github.com/cocomhub/download-manager/testutil/assert"
)

// newAPIServerWithMockWithCollection creates a server with mock objects that have
// collection_id and collection_title metadata. After seeding, assigns sequential IDs.
func newAPIServerWithMockWithCollection(t *testing.T, taskID string, objectCount int, collectionID string, titles []string) (*Server, *config.Config) {
	t.Helper()

	if len(titles) < objectCount {
		titles = make([]string, objectCount)
		for i := range titles {
			titles[i] = "Title " + string(rune('A'+i))
		}
	}

	cfg := &config.Config{
		Runtime: config.Runtime{
			Mode: config.RunModeFull,
			Download: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{
				Enabled: true,
			},
			Scheduler: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{
				Enabled: true,
			},
		},
		Server: config.Server{
			WorkDir:         t.TempDir(),
			DownloadRootDir: t.TempDir(),
		},
		Downloader: config.Downloader{
			GlobalConcurrent: 5,
			MaxRetries:       2,
		},
		Tasks: []config.Task{
			{
				ID:      taskID,
				Type:    "mock",
				SaveDir: t.TempDir(),
				Storage: config.StorageConfig{Type: "memory"},
				Extra: map[string]any{
					"mock_rules": []any{
						map[string]any{
							"url_template": "http://mock-download/collection-{n}.bin",
							"count":        objectCount,
							"metadata": map[string]any{
								"collection_id":    collectionID,
								"collection_title": titles[0],
							},
						},
					},
					"refresh_interval": 0,
				},
			},
		},
	}

	mgr := manager.NewManager(cfg)
	srv := NewServer(mgr)
	return srv, cfg
}

// assignObjectIDs assigns sequential IDs to all objects in the task's storage.
func assignObjectIDs(t *testing.T, srv *Server, taskType string) {
	t.Helper()
	mgr := srv.mgr
	task := mgr.FirstTaskOfType(taskType)
	if task == nil {
		t.Fatalf("task type %q not found", taskType)
	}
	st := task.Storage()
	if st == nil {
		t.Fatalf("storage not found for task %q", task.ID())
	}
	objects, err := st.Search(&core.StorageQuery{
		Filter: core.StorageFilter{
			TaskIDs: []string{task.ID()},
		},
		Limit: 0,
	})
	if err != nil {
		t.Fatalf("search objects: %v", err)
	}
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].URL < objects[j].URL
	})
	for i, obj := range objects {
		obj.SetID(int64(i + 1))
		if err := st.Update(obj); err != nil {
			t.Fatalf("update object %d: %v", i, err)
		}
	}
}

// getObjectIDByIndex returns the ID of the i-th object (sorted by URL) in the task.
func getObjectIDByIndex(t *testing.T, srv *Server, taskType string, index int) int64 {
	t.Helper()
	mgr := srv.mgr
	task := mgr.FirstTaskOfType(taskType)
	if task == nil {
		t.Fatalf("task type %q not found", taskType)
	}
	st := task.Storage()
	if st == nil {
		t.Fatalf("storage not found")
	}
	objects, err := st.Search(&core.StorageQuery{
		Filter: core.StorageFilter{
			TaskIDs: []string{task.ID()},
		},
		Limit: 0,
	})
	if err != nil {
		t.Fatalf("search objects: %v", err)
	}
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].URL < objects[j].URL
	})
	if index < 0 || index >= len(objects) {
		t.Fatalf("index %d out of range (len=%d)", index, len(objects))
	}
	return objects[index].GetID()
}

// TestAPI_GetCollection_Success verifies GET /api/objects/{type}/{id}/collection
// returns collection objects sorted by collection_title.
func TestAPI_GetCollection_Success(t *testing.T) {
	srv, _ := newAPIServerWithMockWithCollection(t, "mock-coll-succ", 3, "coll-1", nil)
	r := srv.Router()

	done := startAPIManager(t, srv)
	// Wait for task to seed objects.
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-coll-succ")
		if rr.Code != http.StatusOK {
			return false
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			return false
		}
		total, _ := result["total"].(float64)
		return total >= 3
	}, 3*time.Second, 50*time.Millisecond, "wait for task to seed 3 objects")

	// Assign IDs (1, 2, 3) to the objects.
	assignObjectIDs(t, srv, "mock")

	// Get the ID of the first object (index 0).
	firstID := getObjectIDByIndex(t, srv, "mock", 0)

	rr := doJSONGet(t, r, fmt.Sprintf("/api/objects/mock/%d/collection", firstID))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/objects/mock/%d/collection returned %d, want 200: %s",
			firstID, rr.Code, rr.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	total, _ := result["total"].(float64)
	if total != 3 {
		t.Errorf("expected total=3, got %.0f", total)
	}

	objects, ok := result["objects"].([]any)
	if !ok {
		t.Fatal("expected objects array in response")
	}
	if len(objects) != 3 {
		t.Fatalf("expected 3 objects, got %d", len(objects))
	}

	_ = done
}

// TestAPI_GetCollection_NoCollection verifies that an object without
// collection_id returns an empty collection (200 with empty objects).
func TestAPI_GetCollection_NoCollection(t *testing.T) {
	srv, _ := newAPIServerWithMockWithCollection(t, "mock-coll-none", 1, "", nil)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-coll-none")
		if rr.Code != http.StatusOK {
			return false
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			return false
		}
		total, _ := result["total"].(float64)
		return total >= 1
	}, 3*time.Second, 50*time.Millisecond, "wait for task to seed objects")

	// Assign ID to the object.
	assignObjectIDs(t, srv, "mock")
	firstID := getObjectIDByIndex(t, srv, "mock", 0)

	rr := doJSONGet(t, r, fmt.Sprintf("/api/objects/mock/%d/collection", firstID))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/objects/mock/%d/collection returned %d, want 200: %s",
			firstID, rr.Code, rr.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	total, _ := result["total"].(float64)
	if total != 0 {
		t.Errorf("expected total=0 for no collection, got %.0f", total)
	}
	objects, _ := result["objects"].([]any)
	if len(objects) != 0 {
		t.Errorf("expected empty objects array, got %d", len(objects))
	}

	_ = done
}

// TestAPI_GetCollection_InvalidID verifies 400 for non-positive integer or non-numeric ID.
func TestAPI_GetCollection_InvalidID(t *testing.T) {
	srv, _ := newAPIServerWithMockWithCollection(t, "mock-coll-invalid", 1, "coll-x", nil)
	r := srv.Router()

	// Non-numeric
	rr := doJSONGet(t, r, "/api/objects/mock/abc/collection")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("GET /api/objects/mock/abc/collection returned %d, want 400", rr.Code)
	}

	// Non-positive
	rr2 := doJSONGet(t, r, "/api/objects/mock/0/collection")
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("GET /api/objects/mock/0/collection returned %d, want 400", rr2.Code)
	}

	rr3 := doJSONGet(t, r, "/api/objects/mock/-1/collection")
	if rr3.Code != http.StatusBadRequest {
		t.Fatalf("GET /api/objects/mock/-1/collection returned %d, want 400", rr3.Code)
	}
}

// TestAPI_GetCollection_NotFoundType verifies 404 for unknown task type.
func TestAPI_GetCollection_NotFoundType(t *testing.T) {
	srv, _ := newAPIServerWithMockWithCollection(t, "mock-coll-type404", 1, "coll-x", nil)
	r := srv.Router()

	rr := doJSONGet(t, r, "/api/objects/nonexistent/1/collection")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET /api/objects/nonexistent/1/collection returned %d, want 404", rr.Code)
	}
}

// TestAPI_GetCollection_NotFoundID verifies 404 for unknown object ID.
func TestAPI_GetCollection_NotFoundID(t *testing.T) {
	srv, _ := newAPIServerWithMockWithCollection(t, "mock-coll-id404", 1, "coll-x", nil)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-coll-id404")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for task")

	// ID=99999 should not exist.
	rr := doJSONGet(t, r, "/api/objects/mock/99999/collection")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET /api/objects/mock/99999/collection returned %d, want 404", rr.Code)
	}

	_ = done
}
