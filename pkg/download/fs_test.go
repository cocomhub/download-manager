// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathRelative(t *testing.T) {
	result, err := ResolvePath("/root", "sub/file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.FromSlash("/root/sub/file.txt")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestResolvePathOutsideRoot(t *testing.T) {
	_, err := ResolvePath("/root", "../outside")
	if err == nil {
		t.Fatal("expected error for path outside root")
	}
}

func TestResolvePathEmptyRoot(t *testing.T) {
	result, err := ResolvePath("", "/some/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "/some/path" {
		t.Errorf("expected '/some/path', got %q", result)
	}
}

func TestResolvePathAbsWithinRoot(t *testing.T) {
	// Create a temp dir to use as root
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create sub dir: %v", err)
	}

	result, err := ResolvePath(dir, subDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != subDir {
		t.Errorf("expected %q, got %q", subDir, result)
	}
}

func TestResolvePathAbsOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(dir, "..", "outside")

	_, err := ResolvePath(dir, outside)
	if err == nil {
		t.Fatal("expected error for absolute path outside root")
	}
}

func TestEnsureDir(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "sub", "nested", "file.txt")

	if err := EnsureDir(testFile); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Dir(testFile)); os.IsNotExist(err) {
		t.Fatal("directory was not created")
	}
}

func TestEnsureDirExisting(t *testing.T) {
	dir := t.TempDir()
	// Should not error on existing directory
	if err := EnsureDir(filepath.Join(dir, "file.txt")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsWithinRoot(t *testing.T) {
	tests := []struct {
		root   string
		path   string
		within bool
	}{
		{"/root", "/root/sub/file.txt", true},
		{"/root", "/root", true},
		{"/root", "/other/file.txt", false},
		{"/root", "/root/../outside", false},
	}

	for _, tt := range tests {
		result := isWithinRoot(tt.root, tt.path)
		if result != tt.within {
			t.Errorf("isWithinRoot(%q, %q) = %v, want %v", tt.root, tt.path, result, tt.within)
		}
	}
}
