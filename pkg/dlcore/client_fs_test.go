// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dlcore

import (
	"path/filepath"
	"testing"
)

func TestResolvePath(t *testing.T) {
	root := t.TempDir()

	rel := filepath.Join("a", "b.txt")
	got, err := ResolvePath(root, rel)
	if err != nil {
		t.Fatalf("ResolvePath(%q) error: %v", rel, err)
	}
	want := filepath.Join(root, "a", "b.txt")
	if got != want {
		t.Fatalf("ResolvePath(%q) = %q, want %q", rel, got, want)
	}

	insideAbs := filepath.Join(root, "c", "d.txt")
	got, err = ResolvePath(root, insideAbs)
	if err != nil {
		t.Fatalf("ResolvePath(abs inside root) error: %v", err)
	}
	if got != insideAbs {
		t.Fatalf("ResolvePath(abs inside root) = %q, want %q", got, insideAbs)
	}

	sep := string(filepath.Separator)
	outsideRel := ".." + sep + "e.txt"
	if _, err := ResolvePath(root, outsideRel); err == nil {
		t.Fatalf("ResolvePath(%q) want error, got nil", outsideRel)
	}
}

func TestIsWithinRoot(t *testing.T) {
	root := t.TempDir()

	if !IsWithinRoot(root, root) {
		t.Fatalf("IsWithinRoot(root, root) = false, want true")
	}
	sub := filepath.Join(root, "child", "file.txt")
	if !IsWithinRoot(root, sub) {
		t.Fatalf("IsWithinRoot(root, sub) = false, want true")
	}
	outside := filepath.Join(root, "..", "x.txt")
	if IsWithinRoot(root, outside) {
		t.Fatalf("IsWithinRoot(root, outside) = true, want false")
	}
}
