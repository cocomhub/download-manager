// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"testing"
)

func TestTryGetMd5WeakEtag(t *testing.T) {
	// Weak ETags have the format W/"32hexchars" (36 chars total with quotes).
	headers := map[string]string{
		"Etag": `W/"5d41402abc4b2a76b9719d911017c592"`, // 36 chars (weak ETag)
	}
	result := TryGetMd5(headers)
	if result != "5d41402abc4b2a76b9719d911017c592" {
		t.Errorf("expected MD5 hex from weak ETag, got %q", result)
	}
}

func TestTryGetMd5WeakEtagLowercase(t *testing.T) {
	// Test lowercase w/ prefix
	headers := map[string]string{
		"Etag": `w/"5d41402abc4b2a76b9719d911017c592"`,
	}
	result := TryGetMd5(headers)
	if result != "5d41402abc4b2a76b9719d911017c592" {
		t.Errorf("expected MD5 hex from lowercase weak ETag, got %q", result)
	}
}

func TestTryGetMd5StrongEtag(t *testing.T) {
	headers := map[string]string{
		"Etag": `"5d41402abc4b2a76b9719d911017c592"`,
	}
	result := TryGetMd5(headers)
	if result != "5d41402abc4b2a76b9719d911017c592" {
		t.Errorf("expected MD5 hex from strong ETag, got %q", result)
	}
}

func TestTryGetMd5InvalidEtag(t *testing.T) {
	headers := map[string]string{
		"Etag": `"not-a-valid-md5"`,
	}
	result := TryGetMd5(headers)
	if result != "" {
		t.Errorf("expected empty for invalid ETag, got %q", result)
	}
}
