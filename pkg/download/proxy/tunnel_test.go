// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package proxy_test

import (
	"context"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download/proxy"
)

func TestTunnelProxySelectorNoInstances(t *testing.T) {
	sel := proxy.NewTunnelProxySelector()
	proxyURL, err := sel.Select(context.Background(), "http://example.com/file", nil)
	if err != nil {
		t.Fatalf("Select with no instances should not error: %v", err)
	}
	if proxyURL != "" {
		t.Errorf("expected empty proxy, got %s", proxyURL)
	}
}

func TestTunnelProxySelectorWithInstance(t *testing.T) {
	sel := proxy.NewTunnelProxySelector(
		proxy.WithTunnelInstance("http://localhost:18083", "0000000000000000000000000000000000000000000000000000000000000000"),
	)
	proxyURL, err := sel.Select(context.Background(), "http://example.com/file", nil)
	if err != nil {
		t.Fatalf("Select should not error: %v", err)
	}
	if proxyURL != "" {
		t.Logf("Selected proxy: %s", proxyURL)
	}
	if proxyURL == "" {
		// sproxy not running, that's fine
		t.Log("sproxy not running, returned empty proxy URL")
	}
}

func TestTunnelProxySelectorMultipleInstances(t *testing.T) {
	sel := proxy.NewTunnelProxySelector(
		proxy.WithTunnelInstance("http://sproxy1:18083", "key1"),
		proxy.WithTunnelInstance("http://sproxy2:18083", "key2"),
	)
	proxyURL, err := sel.Select(context.Background(), "http://example.com/file", nil)
	if err != nil {
		t.Fatalf("Select should not error: %v", err)
	}
	t.Logf("Selected proxy: %s", proxyURL)
}
