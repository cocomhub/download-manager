// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"context"
	"sync"
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

func TestDefaultSelectorConcurrentAddAndMatch(t *testing.T) {
	sel := download.NewDefaultSelector()

	// Register extractors
	ext1 := &mockExtractorForSelector{name: "ext1", match: func(_ context.Context, url string) bool { return url == "http://example.com/video" }}
	ext2 := &mockExtractorForSelector{name: "ext2", match: func(_ context.Context, url string) bool { return url == "http://example.com/audio" }}

	sel.AddExtractor(ext1)
	sel.AddExtractor(ext2)

	// Concurrently add extractors and match
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ex := &mockExtractorForSelector{
				name:  "concurrent",
				match: func(_ context.Context, url string) bool { return false },
			}
			sel.AddExtractor(ex)

			// Match concurrently
			matched := sel.MatchExtractor(t.Context(), "http://example.com/video", nil)
			if matched == nil {
				t.Error("expected to match ext1")
			}
		}(i)
	}
	wg.Wait()

	// Verify ext1 still matches
	matched := sel.MatchExtractor(t.Context(), "http://example.com/video", nil)
	if matched == nil {
		t.Fatal("expected to match ext1 after concurrent operations")
	}
	if matched.Name() != "ext1" {
		t.Errorf("expected ext1, got %s", matched.Name())
	}
}

func TestDefaultSelectorMatchByName(t *testing.T) {
	sel := download.NewDefaultSelector()

	ext1 := &mockExtractorForSelector{name: "hls", match: func(_ context.Context, url string) bool { return true }}
	ext2 := &mockExtractorForSelector{name: "http", match: func(_ context.Context, url string) bool { return true }}

	sel.AddExtractor(ext1)
	sel.AddExtractor(ext2)

	// Match by hint name
	matched := sel.MatchExtractor(t.Context(), "http://example.com/stream.m3u8", &download.DownloadHint{Extractor: "hls"})
	if matched == nil {
		t.Fatal("expected to match hls extractor by hint name")
	}
	if matched.Name() != "hls" {
		t.Errorf("expected hls, got %s", matched.Name())
	}

	// Match by hint name for second extractor
	matched = sel.MatchExtractor(t.Context(), "http://example.com/file", &download.DownloadHint{Extractor: "http"})
	if matched == nil {
		t.Fatal("expected to match http extractor by hint name")
	}
	if matched.Name() != "http" {
		t.Errorf("expected http, got %s", matched.Name())
	}

	// Non-existent hint name should fall through to match by URL
	matched = sel.MatchExtractor(t.Context(), "http://example.com/file", &download.DownloadHint{Extractor: "nonexistent"})
	if matched == nil {
		t.Fatal("expected to match an extractor via fallback")
	}
}

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
	sel := download.NewDefaultSelector()

	matched := sel.MatchExtractor(t.Context(), "http://example.com/file", nil)
	if matched != nil {
		t.Errorf("expected nil, got %s", matched.Name())
	}
}

func TestDefaultSelectorMatchNilHint(t *testing.T) {
	sel := download.NewDefaultSelector()

	ext1 := &mockExtractorForSelector{name: "http", match: func(_ context.Context, url string) bool { return true }}
	sel.AddExtractor(ext1)

	// nil hint should still work
	matched := sel.MatchExtractor(t.Context(), "http://example.com/file", nil)
	if matched == nil {
		t.Fatal("expected to match with nil hint")
	}
	if matched.Name() != "http" {
		t.Errorf("expected http, got %s", matched.Name())
	}
}
