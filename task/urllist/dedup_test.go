// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package urllist

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/task"
)

// TestDuplicateFilename_NoDuplicates verifies that a normal URL list
// produces expected filenames.
func TestDuplicateFilename_NoDuplicates(t *testing.T) {
	task, err := task.NewTask(&config.Task{
		ID:      "dedup-no",
		Type:    TaskType,
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
		Extra: map[string]any{
			"urls": []string{
				"https://example.com/file-a.dat",
				"https://example.com/file-b.dat",
				"https://example.com/file-c.dat",
			},
		},
	})
	if err != nil {
		t.Fatalf("new task err: %s", err)
	}

	objs := task.(*Task).GetAllObjects(true)
	if len(objs) != 3 {
		t.Fatalf("expected 3 objects, got %d", len(objs))
	}

	expectedSet := map[string]bool{
		"file-a.dat": false,
		"file-b.dat": false,
		"file-c.dat": false,
	}
	if len(objs) != len(expectedSet) {
		t.Fatalf("expected %d objects, got %d", len(expectedSet), len(objs))
	}
	for _, obj := range objs {
		matched := false
		for exp := range expectedSet {
			if stringsSuffix(obj.SavePath, exp) {
				if expectedSet[exp] {
					t.Errorf("duplicate filename: %s for %s", exp, obj.SavePath)
				}
				expectedSet[exp] = true
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("unexpected SavePath: %s", obj.SavePath)
		}
	}
}

// TestDuplicateFilename_Basic verifies that duplicate filenames get suffixed.
func TestDuplicateFilename_Basic(t *testing.T) {
	task, err := task.NewTask(&config.Task{
		ID:      "dedup-1",
		Type:    TaskType,
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
		Extra: map[string]any{
			"urls": []string{
				"https://a.com/file.dat",
				"https://b.com/file.dat",
			},
		},
	})
	if err != nil {
		t.Fatalf("new task err: %s", err)
	}

	objs := task.(*Task).GetAllObjects(true)
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objs))
	}

	path0 := objs[0].SavePath
	path1 := objs[1].SavePath
	if path0 == path1 {
		t.Errorf("expected different SavePaths for duplicate filenames, got both %q", path0)
	}
	if !stringsSuffix(path0, "file.dat") && !stringsSuffix(path1, "file.dat") {
		t.Errorf("expected at least one SavePath to end with file.dat, got %q and %q", path0, path1)
	}
	if !stringsSuffix(path0, "file_1.dat") && !stringsSuffix(path1, "file_1.dat") {
		t.Errorf("expected one SavePath to end with file_1.dat, got %q and %q", path0, path1)
	}
}

// TestDuplicateFilename_Extreme verifies many duplicates of the same base name
// with different URLs (so each creates an object, but filename dedup kicks in).
func TestDuplicateFilename_Extreme(t *testing.T) {
	urls := make([]string, 20)
	for i := range 20 {
		urls[i] = fmt.Sprintf("https://example.com/%d/file.dat", i)
	}

	task, err := task.NewTask(&config.Task{
		ID:      "dedup-extreme",
		Type:    TaskType,
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
		Extra: map[string]any{
			"urls": urls,
		},
	})
	if err != nil {
		t.Fatalf("new task err: %s", err)
	}

	objs := task.(*Task).GetAllObjects(true)
	if len(objs) != 20 {
		t.Fatalf("expected 20 objects, got %d", len(objs))
	}

	// All filenames should be unique
	seen := make(map[string]bool)
	for _, obj := range objs {
		if seen[obj.SavePath] {
			t.Errorf("duplicate SavePath: %s", obj.SavePath)
		}
		seen[obj.SavePath] = true
	}
}

// TestDuplicateFilename_MixedExt verifies duplicates with different extensions
// are handled independently.
func TestDuplicateFilename_MixedExt(t *testing.T) {
	task, err := task.NewTask(&config.Task{
		ID:      "dedup-mix",
		Type:    TaskType,
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
		Extra: map[string]any{
			"urls": []string{
				"https://a.com/video.mp4",
				"https://b.com/video.mp4",
				"https://c.com/video.webm",
				"https://d.com/video.mp4",
			},
		},
	})
	if err != nil {
		t.Fatalf("new task err: %s", err)
	}

	objs := task.(*Task).GetAllObjects(true)
	if len(objs) != 4 {
		t.Fatalf("expected 4 objects, got %d", len(objs))
	}

	seen := make(map[string]bool)
	for _, obj := range objs {
		if seen[obj.SavePath] {
			t.Errorf("duplicate SavePath: %s", obj.SavePath)
		}
		seen[obj.SavePath] = true
	}
}

// TestEmptyURLList verifies empty URL list doesn't panic and produces 0 objects.
func TestEmptyURLList(t *testing.T) {
	task, err := task.NewTask(&config.Task{
		ID:      "empty",
		Type:    TaskType,
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
		Extra: map[string]any{
			"urls": []string{},
		},
	})
	if err != nil {
		t.Fatalf("new task err: %s", err)
	}

	objs := task.(*Task).GetAllObjects(true)
	if len(objs) != 0 {
		t.Errorf("expected 0 objects for empty URL list, got %d", len(objs))
	}
}

// TestNoExtraURLs verifies that a urllist task without urls extra produces 0 objects.
func TestNoExtraURLs(t *testing.T) {
	task, err := task.NewTask(&config.Task{
		ID:      "no-urls",
		Type:    TaskType,
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
	})
	if err != nil {
		t.Fatalf("new task err: %s", err)
	}

	objs := task.(*Task).GetAllObjects(true)
	if len(objs) != 0 {
		t.Errorf("expected 0 objects for no URLs, got %d", len(objs))
	}
}

// stringsSuffix is a helper to check if s ends with suffix.
func stringsSuffix(s, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

// TestExtraURLs_AnySlice verifies that Extra["urls"] can be []any with strings.
func TestExtraURLs_AnySlice(t *testing.T) {
	task, err := task.NewTask(&config.Task{
		ID:      "any-slice",
		Type:    TaskType,
		SaveDir: t.TempDir(),
		Storage: config.StorageConfig{Type: "memory"},
		Extra: map[string]any{
			"urls": []any{
				"https://a.com/file1.dat",
				"https://b.com/file2.dat",
			},
		},
	})
	if err != nil {
		t.Fatalf("new task err: %s", err)
	}

	objs := task.(*Task).GetAllObjects(true)
	if len(objs) != 2 {
		t.Errorf("expected 2 objects from []any, got %d", len(objs))
	}
}
