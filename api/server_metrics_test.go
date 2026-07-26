// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/manager"
	"github.com/cocomhub/download-manager/testutil/assert"
)

// newAPIServerWithMockWithTags creates a server with mock objects that have
// tags in their Extra map for aggregate filtering tests.
func newAPIServerWithMockWithTags(t *testing.T, taskID string, objectCount int) (*Server, *config.Config) {
	t.Helper()

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
							"url_template": "http://mock-download/tag-{n}.bin",
							"count":        objectCount,
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

// seedTagsIntoObjects writes tags into each object's Extra map after the manager
// has seeded them. This simulates what a real task's storage backend would have.
func seedTagsIntoObjects(t *testing.T, srv *Server, taskType string, tagsByURL map[string][]string) {
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
	for _, obj := range objects {
		if tags, ok := tagsByURL[obj.URL]; ok {
			obj.Lock()
			if obj.Extra == nil {
				obj.Extra = make(map[string]any)
			}
			obj.Extra["tags"] = tags
			obj.Unlock()
			if err := st.Update(obj); err != nil {
				t.Fatalf("update object %s: %v", obj.URL, err)
			}
		}
	}
}

// assignIDsAndSeedTags is a convenience that assigns IDs to objects by URL
// order and seeds tags into them.
func assignIDsAndSeedTags(t *testing.T, srv *Server, taskType string, tagsByURL map[string][]string) {
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
	for i, obj := range objects {
		obj.SetID(int64(i + 1))
		if tags, ok := tagsByURL[obj.URL]; ok {
			obj.Lock()
			if obj.Extra == nil {
				obj.Extra = make(map[string]any)
			}
			obj.Extra["tags"] = tags
			obj.Unlock()
		}
		if err := st.Update(obj); err != nil {
			t.Fatalf("update object %d: %v", i, err)
		}
	}
}

// TestAPI_AggregateWithTags verifies the /api/aggregate endpoint with tags filtering.
func TestAPI_AggregateWithTags(t *testing.T) {
	srv, _ := newAPIServerWithMockWithTags(t, "mock-tag-agg", 5)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-tag-agg")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for task to seed objects")

	// Seed tags: objects 1-3 have "action", object 4 has "comedy", object 5 has "drama"
	tagsByURL := map[string][]string{
		"http://mock-download/tag-0.bin": {"action", "thriller"},
		"http://mock-download/tag-1.bin": {"action", "adventure"},
		"http://mock-download/tag-2.bin": {"action"},
		"http://mock-download/tag-3.bin": {"comedy"},
		"http://mock-download/tag-4.bin": {"drama"},
	}
	assignIDsAndSeedTags(t, srv, "mock", tagsByURL)

	t.Run("tags filter tag_mode=any", func(t *testing.T) {
		rr := doJSONGet(t, r, "/api/aggregate?tags=action&tag_mode=any")
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /api/aggregate?tags=action returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		objects, _ := result["objects"].([]any)
		// Should match 3 objects with "action" tag
		if len(objects) != 3 {
			t.Errorf("expected 3 objects with tag=action, got %d", len(objects))
		}
	})

	t.Run("tags filter tag_mode=all matches all", func(t *testing.T) {
		rr := doJSONGet(t, r, "/api/aggregate?tags=action,thriller&tag_mode=all")
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /api/aggregate?tags=action,thriller&tag_mode=all returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		objects, _ := result["objects"].([]any)
		// Only object 0 has both "action" and "thriller"
		if len(objects) != 1 {
			t.Errorf("expected 1 object with both action and thriller, got %d", len(objects))
		}
	})

	t.Run("tags filter tag_mode=all partial match", func(t *testing.T) {
		rr := doJSONGet(t, r, "/api/aggregate?tags=action,drama&tag_mode=all")
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /api/aggregate?tags=action,drama&tag_mode=all returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		objects, _ := result["objects"].([]any)
		// No object has both "action" and "drama"
		if len(objects) != 0 {
			t.Errorf("expected 0 objects with both action and drama, got %d", len(objects))
		}
	})

	t.Run("tags empty returns all objects", func(t *testing.T) {
		rr := doJSONGet(t, r, "/api/aggregate")
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /api/aggregate returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		total, _ := result["total"].(float64)
		if total != 5 {
			t.Errorf("expected total=5, got %.0f", total)
		}
	})

	_ = done
}

// TestAPI_AggregateWithExcludeIDs verifies the /api/aggregate endpoint with exclude_ids parameter.
func TestAPI_AggregateWithExcludeIDs(t *testing.T) {
	srv, _ := newAPIServerWithMockWithTags(t, "mock-excl-agg", 5)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-excl-agg")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for task to seed objects")

	// Assign IDs to all objects
	assignIDsAndSeedTags(t, srv, "mock", nil)

	t.Run("exclude one id", func(t *testing.T) {
		rr := doJSONGet(t, r, "/api/aggregate?exclude_ids=1")
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /api/aggregate?exclude_ids=1 returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		total, _ := result["total"].(float64)
		if total != 4 {
			t.Errorf("expected total=4 after excluding id=1, got %.0f", total)
		}
	})

	t.Run("exclude multiple ids", func(t *testing.T) {
		rr := doJSONGet(t, r, "/api/aggregate?exclude_ids=1,2,3")
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /api/aggregate?exclude_ids=1,2,3 returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		total, _ := result["total"].(float64)
		if total != 2 {
			t.Errorf("expected total=2 after excluding 3 ids, got %.0f", total)
		}
	})

	t.Run("exclude all ids returns empty", func(t *testing.T) {
		rr := doJSONGet(t, r, "/api/aggregate?exclude_ids=1,2,3,4,5")
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /api/aggregate?exclude_ids=1,2,3,4,5 returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		total, _ := result["total"].(float64)
		if total != 0 {
			t.Errorf("expected total=0 after excluding all, got %.0f", total)
		}
	})

	t.Run("sort=random does not error", func(t *testing.T) {
		rr := doJSONGet(t, r, "/api/aggregate?sort=random")
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /api/aggregate?sort=random returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		objects, _ := result["objects"].([]any)
		if len(objects) != 5 {
			t.Errorf("expected 5 objects with sort=random, got %d", len(objects))
		}
	})

	t.Run("sort=tag_match_desc does not error", func(t *testing.T) {
		rr := doJSONGet(t, r, "/api/aggregate?sort=tag_match_desc")
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /api/aggregate?sort=tag_match_desc returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		objects, _ := result["objects"].([]any)
		// Should still return objects
		if len(objects) == 0 {
			t.Errorf("expected non-empty result with sort=tag_match_desc")
		}
	})

	_ = done
}

// TestAPI_AggregateWithTagsAndExcludeIDs verifies combined tags + exclude_ids filtering.
func TestAPI_AggregateWithTagsAndExcludeIDs(t *testing.T) {
	srv, _ := newAPIServerWithMockWithTags(t, "mock-comb-agg", 5)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-comb-agg")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for task to seed objects")

	// Assign IDs and seed tags: objects 0,1,2 have "action"
	tagsByURL := map[string][]string{
		"http://mock-download/tag-0.bin": {"action"},
		"http://mock-download/tag-1.bin": {"action"},
		"http://mock-download/tag-2.bin": {"action"},
		"http://mock-download/tag-3.bin": {"comedy"},
		"http://mock-download/tag-4.bin": {"drama"},
	}
	assignIDsAndSeedTags(t, srv, "mock", tagsByURL)

	t.Run("tags + exclude_ids combined", func(t *testing.T) {
		// tags=action should match 3 objects, exclude_ids=1,2 should leave 1
		rr := doJSONGet(t, r, "/api/aggregate?tags=action&tag_mode=any&exclude_ids=1,2")
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /api/aggregate with combined params returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		total, _ := result["total"].(float64)
		// Object 3 (ID=3) has "action" and is not excluded → total=1
		if total != 1 {
			t.Errorf("expected total=1 after tags=action + exclude_ids=1,2, got %.0f", total)
		}
	})

	_ = done
}

// TestAPI_Aggregate_TagModeAnyVsAll verifies the difference between tag_mode=any and tag_mode=all.
func TestAPI_Aggregate_TagModeAnyVsAll(t *testing.T) {
	srv, _ := newAPIServerWithMockWithTags(t, "mock-mode-agg", 3)
	r := srv.Router()

	done := startAPIManager(t, srv)
	assert.MustEventually(t, func() bool {
		rr := doJSONGet(t, r, "/api/tasks/mock-mode-agg")
		return rr.Code == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "wait for task to seed objects")

	// Object 0: action, comedy; Object 1: action; Object 2: comedy, drama
	tagsByURL := map[string][]string{
		"http://mock-download/tag-0.bin": {"action", "comedy"},
		"http://mock-download/tag-1.bin": {"action"},
		"http://mock-download/tag-2.bin": {"comedy", "drama"},
	}
	assignIDsAndSeedTags(t, srv, "mock", tagsByURL)

	t.Run("tag_mode=any with action,comedy matches objects 0,1,2", func(t *testing.T) {
		rr := doJSONGet(t, r, "/api/aggregate?tags=action,comedy&tag_mode=any")
		if rr.Code != http.StatusOK {
			t.Fatalf("tag_mode=any returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		objects, _ := result["objects"].([]any)
		// All 3 objects have either action or comedy
		if len(objects) != 3 {
			t.Errorf("expected 3 objects with tag_mode=any, got %d", len(objects))
		}
	})

	t.Run("tag_mode=all with action,comedy matches only object 0", func(t *testing.T) {
		rr := doJSONGet(t, r, "/api/aggregate?tags=action,comedy&tag_mode=all")
		if rr.Code != http.StatusOK {
			t.Fatalf("tag_mode=all returned %d, want 200: %s", rr.Code, rr.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		objects, _ := result["objects"].([]any)
		// Only object 0 has both action and comedy
		if len(objects) != 1 {
			t.Errorf("expected 1 object with tag_mode=all, got %d", len(objects))
		}
	})

	_ = done
}
