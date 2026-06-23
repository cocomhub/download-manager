// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"encoding/json"
	"testing"
)

func TestMarshalJSON_NilReceiver(t *testing.T) {
	t.Parallel()

	var o *DownloadObject
	data, err := o.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON on nil receiver should not error: %v", err)
	}
	if string(data) != "null" {
		t.Errorf("expected 'null', got %s", string(data))
	}

	// Verify it's valid JSON
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Errorf("expected valid JSON, got error: %v", err)
	}
}

func TestMarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		obj      *DownloadObject
		wantKeys []string
	}{
		{
			name: "all fields populated",
			obj: &DownloadObject{
				TaskID:   "task-1",
				URL:      "https://example.com/file.zip",
				SavePath: "/downloads/file.zip",
				Status:   StatusCompleted,
				Progress: 100,
				Metadata: map[string]string{"key1": "val1"},
				Extra:    map[string]any{"extra1": "value1", "count": float64(42)},
			},
			wantKeys: []string{"task_id", "url", "save_path", "status", "progress", "metadata", "extra"},
		},
		{
			name: "nil Metadata and Extra fields",
			obj: &DownloadObject{
				TaskID:   "task-2",
				URL:      "https://example.com/video.mp4",
				SavePath: "/downloads/video.mp4",
				Status:   StatusPending,
				Progress: 0,
			},
			wantKeys: []string{"task_id", "url", "save_path", "status", "progress", "metadata", "extra"},
		},
		{
			name: "empty Metadata and Extra fields",
			obj: &DownloadObject{
				TaskID:   "task-3",
				URL:      "https://example.com/doc.pdf",
				SavePath: "/downloads/doc.pdf",
				Status:   StatusFailed,
				Progress: 50,
				Metadata: map[string]string{},
				Extra:    map[string]any{},
			},
			wantKeys: []string{"task_id", "url", "save_path", "status", "progress", "metadata", "extra"},
		},
		{
			name: "empty task_id omitted",
			obj: &DownloadObject{
				URL:      "https://example.com/no-task.zip",
				SavePath: "/downloads/no-task.zip",
				Status:   StatusDownloading,
				Progress: 30,
				Metadata: map[string]string{"source": "test"},
				Extra:    map[string]any{"retry": float64(3)},
			},
			wantKeys: []string{"url", "save_path", "status", "progress", "metadata", "extra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := tt.obj.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON() error = %v", err)
			}
			if len(data) == 0 {
				t.Fatal("MarshalJSON() returned empty data")
			}

			// Verify it's valid JSON and unmarshal to a map
			var result map[string]any
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("output is not valid JSON: %v\nraw: %s", err, string(data))
			}

			// Check expected keys are present
			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("expected key %q not found in JSON output: %s", key, string(data))
				}
			}

			// Verify field values for non-nil fields
			if tt.obj.TaskID != "" {
				if v, ok := result["task_id"]; !ok || v != tt.obj.TaskID {
					t.Errorf("expected task_id=%q, got %v", tt.obj.TaskID, result["task_id"])
				}
			}
			if v, ok := result["url"]; !ok || v != tt.obj.URL {
				t.Errorf("expected url=%q, got %v", tt.obj.URL, result["url"])
			}
			if v, ok := result["save_path"]; !ok || v != tt.obj.SavePath {
				t.Errorf("expected save_path=%q, got %v", tt.obj.SavePath, result["save_path"])
			}
			if v, ok := result["status"]; !ok || v != tt.obj.Status {
				t.Errorf("expected status=%q, got %v", tt.obj.Status, result["status"])
			}
			if v, ok := result["progress"]; !ok || float64(tt.obj.Progress) != v {
				t.Errorf("expected progress=%d, got %v", tt.obj.Progress, result["progress"])
			}
		})
	}
}
