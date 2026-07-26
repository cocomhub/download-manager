// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package hanime

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/task"
)

func TestTask_Standardize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		url          string
		existingID   int64
		wantModified bool
		wantID       int64
	}{
		{
			name:         "extract ID from query param v",
			url:          "https://hanime1.me/watch?v=407014",
			existingID:   0,
			wantModified: true,
			wantID:       407014,
		},
		{
			name:         "existing ID not modified",
			url:          "https://hanime1.me/watch?v=407014",
			existingID:   42,
			wantModified: false,
			wantID:       42,
		},
		{
			name:         "no ID in URL returns empty",
			url:          "https://example.com/no-id",
			existingID:   0,
			wantModified: false,
			wantID:       0,
		},
		{
			name:         "empty URL",
			url:          "",
			existingID:   0,
			wantModified: false,
			wantID:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bt, err := task.NewBaseTask(&config.Task{
				ID:      "test",
				Type:    TaskType,
				SaveDir: t.TempDir(),
				Storage: config.StorageConfig{Type: "memory"},
			}, task.Options{})
			if err != nil {
				t.Fatalf("NewBaseTask failed: %v", err)
			}
			tk := &Task{BaseTask: bt}
			bt.SetSelf(tk)

			obj := &model.DownloadObject{
				URL:      tt.url,
				ID:       tt.existingID,
				Extra:    map[string]any{},
				Metadata: map[string]string{},
			}

			modified, err := tk.Standardize(obj)
			if err != nil {
				t.Fatalf("Standardize() error: %v", err)
			}
			if modified != tt.wantModified {
				t.Errorf("Standardize() modified = %v, want %v", modified, tt.wantModified)
			}
			if obj.ID != tt.wantID {
				t.Errorf("Standardize() obj.ID = %d, want %d", obj.ID, tt.wantID)
			}
		})
	}
}

func TestTask_Standardize_VerifyCalledViaRememberRuntimeObject(t *testing.T) {
	t.Parallel()

	bt, err := task.NewBaseTask(&config.Task{
		ID:      "test",
		Type:    TaskType,
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
	}, task.Options{})
	if err != nil {
		t.Fatalf("NewBaseTask failed: %v", err)
	}
	tk := &Task{BaseTask: bt}
	bt.SetSelf(tk)

	obj := &model.DownloadObject{
		TaskID:   bt.ID(),
		URL:      "https://hanime1.me/watch?v=407014",
		Status:   model.StatusPending,
		Extra:    map[string]any{},
		Metadata: map[string]string{},
	}

	// RememberRuntimeObject should trigger Standardize via the Standardizer hook
	tk.RememberRuntimeObject(obj, false)

	// Verify ID was set by Standardize
	if obj.ID != 407014 {
		t.Errorf("Expected obj.ID = 407014 after RememberRuntimeObject, got %d", obj.ID)
	}
}

func TestGetPlaylistFromObject(t *testing.T) {
	t.Parallel()

	t.Run("nil Extra", func(t *testing.T) {
		obj := &model.DownloadObject{}
		got := getPlaylistFromObject(obj)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("no playlist key", func(t *testing.T) {
		obj := &model.DownloadObject{Extra: map[string]any{}}
		got := getPlaylistFromObject(obj)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("[]map[string]string format", func(t *testing.T) {
		obj := &model.DownloadObject{
			Extra: map[string]any{
				"playlist": []map[string]string{
					{"url": "https://hanime1.me/watch?v=1", "title": "Video 1", "thumb": "thumb1.jpg"},
				},
			},
		}
		got := getPlaylistFromObject(obj)
		if len(got) != 1 {
			t.Fatalf("expected 1 item, got %d", len(got))
		}
		if got[0].href != "https://hanime1.me/watch?v=1" || got[0].title != "Video 1" || got[0].thumbURL != "thumb1.jpg" {
			t.Errorf("item mismatch: href=%q title=%q thumb=%q", got[0].href, got[0].title, got[0].thumbURL)
		}
	})

	t.Run("[]any format", func(t *testing.T) {
		obj := &model.DownloadObject{
			Extra: map[string]any{
				"playlist": []any{
					map[string]any{"url": "https://hanime1.me/watch?v=3", "title": "Video 3", "thumb": "thumb3.jpg"},
				},
			},
		}
		got := getPlaylistFromObject(obj)
		if len(got) != 1 {
			t.Fatalf("expected 1 item, got %d", len(got))
		}
		if got[0].href != "https://hanime1.me/watch?v=3" || got[0].title != "Video 3" || got[0].thumbURL != "thumb3.jpg" {
			t.Errorf("item mismatch: href=%q title=%q thumb=%q", got[0].href, got[0].title, got[0].thumbURL)
		}
	})
}

func TestStandardize_CollectionInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		url               string
		existingID        int64
		existingCollID    string
		existingCollTitle string
		playlist          []map[string]string
		wantModified      bool
		wantCollID        string
		wantCollTitle     string
	}{
		{
			name:       "set collection_id and title from playlist",
			url:        "https://hanime1.me/watch?v=2",
			existingID: 2,
			playlist: []map[string]string{
				{"url": "https://hanime1.me/watch?v=5", "title": "Video 5", "thumb": ""},
				{"url": "https://hanime1.me/watch?v=2", "title": "Video 2", "thumb": ""},
				{"url": "https://hanime1.me/watch?v=1", "title": "Video 1", "thumb": ""},
			},
			wantModified:  true,
			wantCollID:    "1",
			wantCollTitle: "Video 2",
		},
		{
			name:       "dedup by URL",
			url:        "https://hanime1.me/watch?v=2",
			existingID: 2,
			playlist: []map[string]string{
				{"url": "https://hanime1.me/watch?v=1", "title": "Video 1", "thumb": ""},
				{"url": "https://hanime1.me/watch?v=1", "title": "Duplicate", "thumb": ""},
				{"url": "https://hanime1.me/watch?v=3", "title": "Video 3", "thumb": ""},
			},
			wantModified:  true,
			wantCollID:    "1",
			wantCollTitle: "",
		},
		{
			name:          "no playlist",
			url:           "https://hanime1.me/watch?v=2",
			existingID:    2,
			playlist:      nil,
			wantModified:  false,
			wantCollID:    "",
			wantCollTitle: "",
		},
		{
			name:              "existing collection_id preserved",
			url:               "https://hanime1.me/watch?v=2",
			existingID:        2,
			existingCollID:    "5",
			existingCollTitle: "",
			playlist: []map[string]string{
				{"url": "https://hanime1.me/watch?v=1", "title": "Video 1", "thumb": ""},
				{"url": "https://hanime1.me/watch?v=2", "title": "Video 2", "thumb": ""},
			},
			wantModified:  true,
			wantCollID:    "5",
			wantCollTitle: "Video 2",
		},
		{
			name:              "existing collection_title preserved",
			url:               "https://hanime1.me/watch?v=2",
			existingID:        2,
			existingCollID:    "",
			existingCollTitle: "Existing Title",
			playlist: []map[string]string{
				{"url": "https://hanime1.me/watch?v=1", "title": "Video 1", "thumb": ""},
			},
			wantModified:  true,
			wantCollID:    "1",
			wantCollTitle: "Existing Title",
		},
		{
			name:              "all existing not modified",
			url:               "https://hanime1.me/watch?v=2",
			existingID:        2,
			existingCollID:    "5",
			existingCollTitle: "Existing Title",
			playlist: []map[string]string{
				{"url": "https://hanime1.me/watch?v=1", "title": "Video 1", "thumb": ""},
			},
			wantModified:  false,
			wantCollID:    "5",
			wantCollTitle: "Existing Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bt, err := task.NewBaseTask(&config.Task{
				ID:      "test-collection",
				Type:    TaskType,
				SaveDir: t.TempDir(),
				Storage: config.StorageConfig{Type: "memory"},
			}, task.Options{})
			if err != nil {
				t.Fatalf("NewBaseTask failed: %v", err)
			}
			tk := &Task{BaseTask: bt}
			bt.SetSelf(tk)

			extra := map[string]any{}
			if tt.playlist != nil {
				extra["playlist"] = tt.playlist
			}
			obj := &model.DownloadObject{
				URL:      tt.url,
				ID:       tt.existingID,
				Extra:    extra,
				Metadata: map[string]string{},
			}
			if tt.existingCollID != "" {
				obj.Metadata["collection_id"] = tt.existingCollID
			}
			if tt.existingCollTitle != "" {
				obj.Metadata["collection_title"] = tt.existingCollTitle
			}

			modified, err := tk.Standardize(obj)
			if err != nil {
				t.Fatalf("Standardize() error: %v", err)
			}
			if modified != tt.wantModified {
				t.Errorf("Standardize() modified = %v, want %v", modified, tt.wantModified)
			}
			if obj.Metadata["collection_id"] != tt.wantCollID {
				t.Errorf("collection_id = %q, want %q", obj.Metadata["collection_id"], tt.wantCollID)
			}
			if obj.Metadata["collection_title"] != tt.wantCollTitle {
				t.Errorf("collection_title = %q, want %q", obj.Metadata["collection_title"], tt.wantCollTitle)
			}
		})
	}
}
