// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"context"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

// mockExtractorForSelector is a test extractor used for selector testing.
type mockExtractorForSelector struct {
	name  string
	match func(ctx context.Context, url string) bool
}

func (m *mockExtractorForSelector) Name() string { return m.name }
func (m *mockExtractorForSelector) Match(ctx context.Context, url string) bool {
	return m.match(ctx, url)
}
func (m *mockExtractorForSelector) Extract(_ context.Context, _ *download.Request) error { return nil }

func TestDefaultSelectorSelectProxy(t *testing.T) {
	sel := download.NewDefaultSelector()

	// No proxy selector set — should return empty
	proxy, err := sel.SelectProxy(t.Context(), "http://example.com/file", nil)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if proxy != "" {
		t.Errorf("expected empty proxy, got %s", proxy)
	}
}

func TestDefaultSelectorMatchNoExtractors(t *testing.T) {
	// DefaultSelector no longer holds extractors — MatchExtractor always returns nil.
	// Extractor matching is handled by Downloader.matchExtractor fallback.
	sel := download.NewDefaultSelector()

	matched := sel.MatchExtractor(t.Context(), "http://example.com/file", nil)
	if matched != nil {
		t.Errorf("expected nil, got %s", matched.Name())
	}
}
