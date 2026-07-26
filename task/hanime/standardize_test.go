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