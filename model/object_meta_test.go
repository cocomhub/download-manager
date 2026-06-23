// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"sync"
	"testing"
)

func TestGetTags_NilObject(t *testing.T) {
	if tags := (*DownloadObject)(nil).GetTags(); tags != nil {
		t.Fatalf("expected nil tags for nil object")
	}
}

func TestGetTags_FromExtra(t *testing.T) {
	o := &DownloadObject{Extra: map[string]any{"tags": []string{"tag1", "tag2"}}}
	tags := o.GetTags()
	if len(tags) != 2 || tags[0] != "tag1" || tags[1] != "tag2" {
		t.Fatalf("unexpected tags: %v", tags)
	}
}

func TestSetTags_RoundTrip(t *testing.T) {
	o := &DownloadObject{}
	o.SetTags([]string{"a", "b", "c"})
	tags := o.GetTags()
	if len(tags) != 3 || tags[0] != "a" {
		t.Fatalf("unexpected tags: %v", tags)
	}
}

func TestSetTags_NilClears(t *testing.T) {
	o := &DownloadObject{Extra: map[string]any{"tags": []string{"x"}}}
	o.SetTags(nil)
	if _, exists := o.Extra["tags"]; exists {
		t.Fatal("expected tags key to be deleted")
	}
}

func TestPreviewURL_RoundTrip(t *testing.T) {
	o := &DownloadObject{}
	o.SetPreviewURL("http://example.com/preview.jpg")
	if url := o.GetPreviewURL(); url != "http://example.com/preview.jpg" {
		t.Fatalf("unexpected preview_url: %q", url)
	}
}

func TestLocalPreview_RoundTrip(t *testing.T) {
	o := &DownloadObject{}
	o.SetLocalPreview("/path/to/preview.jpg")
	if path := o.GetLocalPreview(); path != "/path/to/preview.jpg" {
		t.Fatalf("unexpected local_preview: %q", path)
	}
}

func TestGroupSize_RoundTrip(t *testing.T) {
	o := &DownloadObject{Extra: map[string]any{"group_size": 5}}
	if n := o.GetGroupSize(); n != 5 {
		t.Fatalf("unexpected group_size: %d", n)
	}
	o.SetGroupSize(10)
	if n := o.GetGroupSize(); n != 10 {
		t.Fatalf("unexpected group_size after set: %d", n)
	}
}

func TestGroupSize_NilReturnsZero(t *testing.T) {
	o := &DownloadObject{}
	if n := o.GetGroupSize(); n != 0 {
		t.Fatalf("expected 0 for nil Extra, got %d", n)
	}
}

func TestMetadataTitle_RoundTrip(t *testing.T) {
	o := &DownloadObject{}
	o.SetMetaTitle("Test Title")
	if title := o.GetMetaTitle(); title != "Test Title" {
		t.Fatalf("unexpected title: %q", title)
	}
}

func TestMetadataDate_RoundTrip(t *testing.T) {
	o := &DownloadObject{}
	o.SetMetaDate("2024-01-15")
	if date := o.GetMetaDate(); date != "2024-01-15" {
		t.Fatalf("unexpected date: %q", date)
	}
}

func TestMetadataDuration_RoundTrip(t *testing.T) {
	o := &DownloadObject{}
	o.SetMetaDuration("25:30")
	if dur := o.GetMetaDuration(); dur != "25:30" {
		t.Fatalf("unexpected duration: %q", dur)
	}
}

func TestMetadataContentGroup_RoundTrip(t *testing.T) {
	o := &DownloadObject{}
	o.SetMetaContentGroup("CLUB-100")
	if g := o.GetMetaContentGroup(); g != "CLUB-100" {
		t.Fatalf("unexpected content_group: %q", g)
	}
}

func TestMetadataTaskType_RoundTrip(t *testing.T) {
	o := &DownloadObject{}
	o.SetMetaTaskType("tktube")
	if tt := o.GetMetaTaskType(); tt != "tktube" {
		t.Fatalf("unexpected task_type: %q", tt)
	}
}

func TestBackwardCompat_ExtraDirectAccess(t *testing.T) {
	// Ensure old code reading Extra["tags"] directly still works after using SetTags
	o := &DownloadObject{}
	o.SetTags([]string{"a", "b"})
	raw, ok := o.Extra["tags"]
	if !ok {
		t.Fatal("expected tags key in Extra")
	}
	tags, ok := raw.([]string)
	if !ok || len(tags) != 2 {
		t.Fatalf("unexpected tags in Extra: %v", tags)
	}
}

func TestBackwardCompat_MetadataDirectAccess(t *testing.T) {
	// Ensure old code reading Metadata["title"] directly still works
	o := &DownloadObject{}
	o.SetMetaTitle("Legacy")
	if o.Metadata[MetadataKeyTitle] != "Legacy" {
		t.Fatalf("expected Metadata title = 'Legacy', got %q", o.Metadata[MetadataKeyTitle])
	}
}

var o *DownloadObject

func TestNilSafety_AllAccessors(t *testing.T) {
	_ = o.GetPreviewURL()
	_ = o.GetLocalPreview()
	_ = o.GetGroupSize()
	_ = o.GetMetaTitle()
	_ = o.GetMetaDate()
	_ = o.GetMetaDuration()
	_ = o.GetMetaContentGroup()
	_ = o.GetMetaTaskType()
	_ = o.GetContentGroup()
	_ = o.GetProgress()
	// All should not panic
}

func TestGetProgress(t *testing.T) {
	o := &DownloadObject{Progress: 42}
	if got := o.GetProgress(); got != 42 {
		t.Fatalf("GetProgress() = %d, want 42", got)
	}

	// Nil object returns 0
	var nilObj *DownloadObject
	if got := nilObj.GetProgress(); got != 0 {
		t.Fatalf("nil GetProgress() = %d, want 0", got)
	}

	// Ensure threading safety: multiple goroutines can read concurrently
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			o.GetProgress()
		})
	}
	wg.Wait()
}

func TestObjectLocking(t *testing.T) {
	o := &DownloadObject{Progress: 100}

	// Lock/Unlock basic contract (access field directly, not via GetProgress
	// which itself acquires the lock, to avoid RWMutex non-reentrancy)
	o.Lock()
	if o.Progress != 100 {
		t.Error("Progress inside Lock should still be accessible")
	}
	o.Unlock()

	// RLock/RUnlock basic contract
	o.RLock()
	if o.Progress != 100 {
		t.Error("Progress inside RLock should still be accessible")
	}
	o.RUnlock()

	// Concurrent read locks should not block each other
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			o.RLock()
			defer o.RUnlock()
			_ = o.Progress
		})
	}
	wg.Wait()
}

func TestGetSetContentGroup(t *testing.T) {
	// Round-trip: set then get
	o := &DownloadObject{}
	o.SetContentGroup("GRP-001")
	if got := o.GetContentGroup(); got != "GRP-001" {
		t.Fatalf("GetContentGroup() = %q, want %q", got, "GRP-001")
	}

	// Overwrite
	o.SetContentGroup("GRP-002")
	if got := o.GetContentGroup(); got != "GRP-002" {
		t.Fatalf("GetContentGroup() after overwrite = %q, want %q", got, "GRP-002")
	}

	// Empty value
	o.SetContentGroup("")
	if got := o.GetContentGroup(); got != "" {
		t.Fatalf("GetContentGroup() after empty set = %q, want empty", got)
	}

	// Nil object
	var nilObj *DownloadObject
	nilObj.SetContentGroup("test") // should not panic
	if got := nilObj.GetContentGroup(); got != "" {
		t.Fatalf("nil GetContentGroup() = %q, want empty", got)
	}

	// Nil Extra returns empty
	o2 := &DownloadObject{}
	if got := o2.GetContentGroup(); got != "" {
		t.Fatalf("empty object GetContentGroup() = %q, want empty", got)
	}
}
