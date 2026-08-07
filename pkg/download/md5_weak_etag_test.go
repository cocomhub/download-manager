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

func TestTryGetMd5WeakEtagNonHexContent(t *testing.T) {
	// Weak ETag with 36 chars but non-hex inner content → should be skipped
	headers := map[string]string{
		"Etag": `W/"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"`, // 36 chars, inner is 32 chars but not hex
	}
	result := TryGetMd5(headers)
	if result != "" {
		t.Errorf("expected empty for weak ETag with non-hex content, got %q", result)
	}
}

func TestTryGetMd5StrongEtagNonHexContent(t *testing.T) {
	// Strong ETag with 34 chars but non-hex inner content → should be skipped
	headers := map[string]string{
		"Etag": `"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"`, // 34 chars, inner is 32 chars but not hex
	}
	result := TryGetMd5(headers)
	if result != "" {
		t.Errorf("expected empty for strong ETag with non-hex content, got %q", result)
	}
}

func TestTryGetMd5EtagWrongLength(t *testing.T) {
	// ETag with wrong inner length (not 32 hex chars) → should return empty
	headers := map[string]string{
		"Etag": `"not-a-valid-md5"`,
	}
	result := TryGetMd5(headers)
	if result != "" {
		t.Errorf("expected empty for short ETag, got %q", result)
	}
}

func TestTryGetMd5EtagInvalidHex(t *testing.T) {
	// 38-char ETag (36 chars inner) with non-hex content → goes through non-standard
	// branch, len(inner) != 32 → returns empty
	headers := map[string]string{
		"Etag": `"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"`, // 36 chars inner, 38 total
	}
	result := TryGetMd5(headers)
	if result != "" {
		t.Errorf("expected empty for non-standard-length ETag with non-hex content, got %q", result)
	}
}
