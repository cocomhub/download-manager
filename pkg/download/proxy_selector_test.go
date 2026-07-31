// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"testing"
)

// ---- StaticProxySelector ----

func TestStaticProxySelectorNoProxies(t *testing.T) {
	tests := []struct {
		name    string
		proxies []string
	}{
		{name: "nil", proxies: nil},
		{name: "empty", proxies: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStaticProxySelector(tt.proxies)
			proxy, err := s.Select(t.Context(), "http://example.com/file.zip", nil)
			if err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
			if proxy != "" {
				t.Errorf("expected empty proxy (direct), got: %s", proxy)
			}
		})
	}
}

func TestStaticProxySelectorWithForceProxy(t *testing.T) {
	s := NewStaticProxySelector([]string{"http://127.0.0.1:1"})
	s.forceProxy = true
	proxy, err := s.Select(t.Context(), "http://example.com/file.zip", nil)
	if err == nil {
		t.Error("expected error when forceProxy and no proxy available")
	}
	if proxy != "" {
		t.Errorf("expected empty proxy on error, got: %s", proxy)
	}
}

// ---- DefaultSelector ----

func TestDefaultSelectorSelectProxy(t *testing.T) {
	t.Run("with proxy selector", func(t *testing.T) {
		mockPS := &mockProxySelector{proxyURL: "http://test-proxy:8080"}
		sel := NewDefaultSelector().WithProxySelector(mockPS)
		proxy, err := sel.SelectProxy(t.Context(), "http://example.com/file", nil)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if proxy != "http://test-proxy:8080" {
			t.Errorf("expected proxy http://test-proxy:8080, got: %s", proxy)
		}
	})

	t.Run("without proxy selector", func(t *testing.T) {
		sel := NewDefaultSelector()
		proxy, err := sel.SelectProxy(t.Context(), "http://example.com/file", nil)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if proxy != "" {
			t.Errorf("expected empty proxy, got: %s", proxy)
		}
	})
}

type mockProxySelector struct {
	proxyURL string
}

func (m *mockProxySelector) Select(_ context.Context, _ string, _ *DownloadHint) (string, error) {
	return m.proxyURL, nil
}
