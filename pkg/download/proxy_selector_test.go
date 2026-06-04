// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// ---- StaticProxySelector ----

func TestStaticProxySelectorNoProxies(t *testing.T) {
	s := NewStaticProxySelector(nil)
	proxy, err := s.Select(context.Background(), "http://example.com/file.zip", nil)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if proxy != "" {
		t.Errorf("expected empty proxy (direct), got: %s", proxy)
	}
}

func TestStaticProxySelectorEmptyProxies(t *testing.T) {
	s := NewStaticProxySelector([]string{})
	proxy, err := s.Select(context.Background(), "http://example.com/file.zip", nil)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if proxy != "" {
		t.Errorf("expected empty proxy (direct), got: %s", proxy)
	}
}

func TestStaticProxySelectorWithForceProxy(t *testing.T) {
	s := NewStaticProxySelector([]string{"http://127.0.0.1:1"})
	s.forceProxy = true
	proxy, err := s.Select(context.Background(), "http://example.com/file.zip", nil)
	if err == nil {
		t.Error("expected error when forceProxy and no proxy available")
	}
	if proxy != "" {
		t.Errorf("expected empty proxy on error, got: %s", proxy)
	}
}

func TestStaticProxySelectorCacheDir(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "proxy_cache")
	s := NewStaticProxySelector([]string{"http://127.0.0.1:1"})
	s.forceProxy = true
	s.cacheDir = cacheDir
	_, err := s.Select(context.Background(), "http://example.com/file.zip", nil)
	if err == nil {
		t.Error("expected error when forceProxy and no proxy available")
	}
	// Verify no panic and the base temp dir still exists
	if _, statErr := os.Stat(tmpDir); statErr != nil {
		t.Errorf("temp dir should still exist: %v", statErr)
	}
}
