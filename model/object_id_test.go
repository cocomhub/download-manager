// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"encoding/json"
	"testing"
)

func TestGetID_NilReceiver(t *testing.T) {
	t.Parallel()

	var o *DownloadObject
	if got := o.GetID(); got != 0 {
		t.Errorf("GetID() on nil receiver = %d, want 0", got)
	}
}

func TestSetID_NilReceiver(t *testing.T) {
	t.Parallel()

	var o *DownloadObject
	o.SetID(42) // should not panic
}

func TestGetSetID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   int64
	}{
		{name: "zero value", id: 0},
		{name: "positive small", id: 1},
		{name: "positive large", id: 999999},
		{name: "negative", id: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			o := &DownloadObject{URL: "https://example.com/file"}
			o.SetID(tt.id)
			if got := o.GetID(); got != tt.id {
				t.Errorf("GetID() = %d, want %d", got, tt.id)
			}
		})
	}
}

func TestGetSetID_ThreadSafe(t *testing.T) {
	t.Parallel()

	o := &DownloadObject{URL: "https://example.com/file"}
	done := make(chan struct{})
	go func() {
		o.SetID(100)
		close(done)
	}()
	_ = o.GetID()
	<-done
	if got := o.GetID(); got != 100 {
		t.Errorf("GetID() after concurrent Set = %d, want 100", got)
	}
}

func TestMarshalJSON_WithID(t *testing.T) {
	t.Parallel()

	o := &DownloadObject{
		TaskID:   "task-1",
		URL:      "https://example.com/file.zip",
		SavePath: "/downloads/file.zip",
		Status:   StatusCompleted,
		Progress: 100,
		ID:       42,
		Metadata: map[string]string{"key1": "val1"},
		Extra:    map[string]any{"extra1": "value1"},
	}

	data, err := o.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, string(data))
	}

	if v, ok := result["id"]; !ok {
		t.Error("expected key 'id' not found in JSON output")
	} else if v != float64(42) {
		t.Errorf("expected id=42, got %v", v)
	}
}

func TestMarshalJSON_IDOmittedWhenZero(t *testing.T) {
	t.Parallel()

	o := &DownloadObject{
		TaskID:   "task-2",
		URL:      "https://example.com/another.zip",
		SavePath: "/downloads/another.zip",
		Status:   StatusPending,
		Progress: 0,
		ID:       0,
		Metadata: map[string]string{},
		Extra:    map[string]any{},
	}

	data, err := o.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, string(data))
	}

	if _, ok := result["id"]; ok {
		t.Error("expected key 'id' to be omitted when zero, but it was present")
	}
}
