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

	if err := os.MkdirAll(filepath.Dir(testFile), 0755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Dir(testFile)); os.IsNotExist(err) {
		t.Fatal("directory was not created")
	}
}

func TestEnsureDirExisting(t *testing.T) {
	dir := t.TempDir()
	// Should not error on existing directory
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, "file.txt")), 0755); err != nil {
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
	if !IsWithinRoot(dir, pathViaLink) {
		t.Errorf("IsWithinRoot(%q, %q) = false, want true", dir, pathViaLink)
	}

	// Test: symlink pointing outside root should return false
	outsideDir := t.TempDir()
	outsideLink := filepath.Join(dir, "outside_link")
	if err := os.Symlink(outsideDir, outsideLink); err != nil {
		t.Skip("symlink not supported:", err)
	}
	if IsWithinRoot(dir, outsideLink) {
		t.Errorf("IsWithinRoot(%q, %q) = true, want false", dir, outsideLink)
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

// TestResolvePathNoFollow 验证 ResolvePathNoFollow 不解析符号链接。
func TestResolvePathNoFollow(t *testing.T) {
	root := t.TempDir()
	result, err := ResolvePathNoFollow(root, "sub/file.txt")
	if err != nil {
		t.Fatalf("ResolvePathNoFollow unexpected error: %v", err)
	}
	expected := filepath.Join(root, "sub/file.txt")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestResolvePathFollowSymlinksFalse 验证 followSymlinks=false 时行为与 ResolvePathNoFollow 一致。
func TestResolvePathFollowSymlinksFalse(t *testing.T) {
	root := t.TempDir()
	result, err := ResolvePath(root, "sub/file.txt", false)
	if err != nil {
		t.Fatalf("ResolvePath(..., false) unexpected error: %v", err)
	}
	expected := filepath.Join(root, "sub/file.txt")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
	_, err = ResolvePath(root, "../outside", false)
	if err == nil {
		t.Fatal("expected error for path outside root with followSymlinks=false")
	}
}

// TestResolvePathFollowSymlinksTrue 验证 followSymlinks=true 时默认行为。
func TestResolvePathFollowSymlinksTrue(t *testing.T) {
	root := t.TempDir()
	result, err := ResolvePath(root, "sub/file.txt", true)
	if err != nil {
		t.Fatalf("ResolvePath(..., true) unexpected error: %v", err)
	}
	expected := filepath.Join(root, "sub/file.txt")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestResolveSymlinksSafe 验证 resolveSymlinksSafe 对不存在路径的处理。
func TestResolveSymlinksSafe(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create sub dir: %v", err)
	}
	nonExistent := filepath.Join(subDir, "nonexistent", "file.txt")
	result, err := resolveSymlinksSafe(nonExistent)
	if err != nil {
		t.Fatalf("resolveSymlinksSafe unexpected error: %v", err)
	}
	if result != nonExistent {
		t.Errorf("expected %q, got %q", nonExistent, result)
	}
	root := filepath.VolumeName(dir) + "\\"
	_, err = resolveSymlinksSafe(root)
	if err != nil {
		t.Fatalf("resolveSymlinksSafe unexpected error for root: %v", err)
	}
}

// TestIsWithinRootNonExistentPath 验证 IsWithinRoot 对 rootDir 中不存在的路径能正确处理。
func TestIsWithinRootNonExistentPath(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create sub dir: %v", err)
	}
	nonExistent := filepath.Join(subDir, "future_file.txt")
	if !IsWithinRoot(dir, nonExistent) {
		t.Errorf("IsWithinRoot(%q, %q) = false, want true", dir, nonExistent)
	}
	outsideNonExistent := filepath.Join(dir, "..", "outside", "future_file.txt")
	if IsWithinRoot(dir, outsideNonExistent) {
		t.Errorf("IsWithinRoot(%q, %q) = true, want false", dir, outsideNonExistent)
	}
	traversalNonExistent := filepath.Join(subDir, "..", "..", "outside", "file.txt")
	if IsWithinRoot(dir, traversalNonExistent) {
		t.Errorf("IsWithinRoot(%q, %q) = true, want false", dir, traversalNonExistent)
	}
}

// TestIsWithinRootNoFollow 验证 followSymlinks=false 时符号链接不被解析。
func TestIsWithinRootNoFollow(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create sub dir: %v", err)
	}
	nonExistent := filepath.Join(subDir, "future_file.txt")
	if !IsWithinRoot(dir, nonExistent, false) {
		t.Errorf("IsWithinRoot(%q, %q, false) = false, want true", dir, nonExistent)
	}
	outside := filepath.Join(dir, "..", "outside", "file.txt")
	if IsWithinRoot(dir, outside, false) {
		t.Errorf("IsWithinRoot(%q, %q, false) = true, want false", dir, outside)
	}
}
