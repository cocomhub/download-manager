// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package scrape

import (
	"context"
	"testing"
)

// mockHooks returns PageHooks that simulate a paginated site with given pages.
// Each page returns pageItems, which are passed through ProcessItems.
// allKnownAfter indicates after how many items allKnown becomes true.
func mockHooks(pages [][]string, allKnownAfter int) PageHooks {
	pageIndex := 0
	return PageHooks{
		BuildPageURL: func(page int) string {
			return "http://mock/page/" + itoa(page)
		},
		RunScraper: func(url string) (string, error) {
			if pageIndex < len(pages) {
				return "<html></html>", nil
			}
			return "", nil
		},
		ParseTotalPages: func(html string) int {
			return len(pages)
		},
		ParsePage: func(html string) (any, error) {
			idx := pageIndex
			pageIndex++
			if idx >= len(pages) {
				return nil, nil
			}
			out := make([]any, len(pages[idx]))
			for i, v := range pages[idx] {
				out[i] = v
			}
			return out, nil
		},
		ProcessItems: func(items any) ([]any, bool) {
			itemsSlice := items.([]any)
			allKnown := pageIndex >= allKnownAfter
			return itemsSlice, allKnown
		},
	}
}

func itoa(i int) string {
	return string(rune('0' + i))
}

func TestDefaultPager_FullMode_AllSuccess(t *testing.T) {
	pages := [][]string{
		{"a1", "a2"},
		{"b1", "b2"},
		{"c1", "c2"},
	}
	hooks := mockHooks(pages, 100) // never allKnown
	ctx := context.Background()
	p := NewDefaultPager()
	result := p.Run(ctx, hooks, Options{Mode: ModeFull, StartPage: 1})

	if !result.AllSucceeded {
		t.Fatalf("Expected all succeeded, got LastFailedPage=%d", result.LastFailedPage)
	}
	if len(result.Items) != 6 {
		t.Fatalf("Expected 6 items, got %d", len(result.Items))
	}
	if result.DetectedPages != 3 {
		t.Fatalf("Expected 3 detected pages, got %d", result.DetectedPages)
	}
}

func TestDefaultPager_FullMode_PartialFail(t *testing.T) {
	hooks := PageHooks{
		BuildPageURL: func(page int) string {
			return "http://mock/page/" + itoa(page)
		},
		RunScraper: func(url string) (string, error) {
			// Page 2 always fails
			if url == "http://mock/page/2" {
				return "", errFake
			}
			return "<html></html>", nil
		},
		ParseTotalPages: func(html string) int { return 3 },
		ParsePage:       func(html string) (any, error) { return []any{"x"}, nil },
		ProcessItems:    func(items any) ([]any, bool) { return items.([]any), false },
	}
	ctx := context.Background()
	p := NewDefaultPager()
	result := p.Run(ctx, hooks, Options{Mode: ModeFull, StartPage: 1, MaxRetries: 3})

	if result.AllSucceeded {
		t.Fatal("Expected AllSucceeded=false due to page 2 failure")
	}
	if result.LastFailedPage <= 0 {
		t.Fatalf("Expected LastFailedPage > 0, got %d", result.LastFailedPage)
	}
}

func TestDefaultPager_IncrementalMode_MaxEmpty(t *testing.T) {
	pageIndex := 0
	hooks := PageHooks{
		BuildPageURL:    func(page int) string { return "http://mock/page/" + itoa(page) },
		RunScraper:      func(url string) (string, error) { return "<html></html>", nil },
		ParseTotalPages: func(html string) int { return 10 },
		ParsePage:       func(html string) (any, error) { pageIndex++; return []any{}, nil },
		ProcessItems:    func(items any) ([]any, bool) { return nil, false },
	}
	ctx := context.Background()
	p := NewDefaultPager()
	result := p.Run(ctx, hooks, Options{Mode: ModeIncremental, StartPage: 1, MaxEmptyPages: 3})

	if !result.AllSucceeded {
		t.Fatalf("Expected AllSucceeded=true (empty pages, no failures), got LastFailedPage=%d", result.LastFailedPage)
	}
	// Should stop at page 4 (3 empty + 1 fails the condition)
	if pageIndex > 4 {
		t.Fatalf("Expected early stop at page ~4, got pageIndex=%d", pageIndex)
	}
}

func TestDefaultPager_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled

	hooks := PageHooks{
		BuildPageURL:    func(page int) string { return "http://mock/page/1" },
		RunScraper:      func(url string) (string, error) { return "<html></html>", nil },
		ParseTotalPages: func(html string) int { return 100 },
		ParsePage:       func(html string) (any, error) { return []any{"a"}, nil },
		ProcessItems:    func(items any) ([]any, bool) { return items.([]any), false },
	}
	p := NewDefaultPager()
	result := p.Run(ctx, hooks, Options{Mode: ModeFull})

	if result.AllSucceeded {
		t.Fatal("Expected AllSucceeded=false for cancelled context")
	}
}
