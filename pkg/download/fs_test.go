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

func TestIsWithinRootSymlink(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real", "target")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatalf("failed to create real dir: %v", err)
	}

	linkDir := filepath.Join(dir, "link")
	// Create symlink: link -> ../real (relative symlink)
	if err := os.Symlink(filepath.Join(dir, "real"), linkDir); err != nil {
		t.Skip("symlink not supported:", err)
	}

	// Test: path through symlink within root should return true
	pathViaLink := filepath.Join(linkDir, "target")
	if !isWithinRoot(dir, pathViaLink) {
		t.Errorf("isWithinRoot(%q, %q) = false, want true", dir, pathViaLink)
	}

	// Test: symlink pointing outside root should return false
	outsideDir := t.TempDir()
	outsideLink := filepath.Join(dir, "outside_link")
	if err := os.Symlink(outsideDir, outsideLink); err != nil {
		t.Skip("symlink not supported:", err)
	}
	if isWithinRoot(dir, outsideLink) {
		t.Errorf("isWithinRoot(%q, %q) = true, want false", dir, outsideLink)
	}
}

func TestResolvePathWithAllowList(t *testing.T) {
	dir := t.TempDir()
	allowedDir := filepath.Join(dir, "allowed")
	otherDir := filepath.Join(dir, "other")
	for _, d := range []string{allowedDir, otherDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}

	// Test: path within allow list should succeed
	result, err := ResolvePathWithAllowList(dir, []string{allowedDir}, filepath.Join(allowedDir, "file.txt"))
	if err != nil {
		t.Fatalf("unexpected error for allowed path: %v", err)
	}
	expected := filepath.Join(allowedDir, "file.txt")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}

	// Test: path outside allow list should be rejected
	_, err = ResolvePathWithAllowList(dir, []string{allowedDir}, filepath.Join(otherDir, "file.txt"))
	if err == nil {
		t.Fatal("expected error for path outside allow list")
	}

	// Test: exact match of allow path should succeed
	result, err = ResolvePathWithAllowList(dir, []string{allowedDir}, allowedDir)
	if err != nil {
		t.Fatalf("unexpected error for exact allow path match: %v", err)
	}
	if result != allowedDir {
		t.Errorf("expected %q, got %q", allowedDir, result)
	}

	// Test: empty allow list behaves like ResolvePath (no restriction)
	result, err = ResolvePathWithAllowList(dir, nil, filepath.Join(otherDir, "file.txt"))
	if err != nil {
		t.Fatalf("unexpected error with empty allow list: %v", err)
	}
	expected = filepath.Join(otherDir, "file.txt")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}

	// Test: path outside root entirely should be rejected regardless of allow list
	_, err = ResolvePathWithAllowList(dir, []string{allowedDir}, filepath.Join(dir, "..", "outside"))
	if err == nil {
		t.Fatal("expected error for path outside root")
	}
}
