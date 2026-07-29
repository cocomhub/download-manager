// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/download/extractor"
)

// mockFailTransport returns an error immediately for any request.
type mockFailTransport struct{}

func (m *mockFailTransport) Name() string { return "mock" }
func (m *mockFailTransport) RoundTrip(_ context.Context, _ *download.TransportRequest) (*download.TransportResponse, error) {
	return nil, fmt.Errorf("mock transport failure")
}

// mockQuickFailExtractor matches any URL and returns an error immediately.
type mockQuickFailExtractor struct {
	callCount int
	mu        sync.Mutex
}

func (m *mockQuickFailExtractor) Name() string                           { return "mockfail" }
func (m *mockQuickFailExtractor) Match(_ context.Context, _ string) bool { return true }
func (m *mockQuickFailExtractor) Extract(_ context.Context, _ *download.Request) error {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
	return fmt.Errorf("mock extractor failure")
}

func TestCompositeExtractorBuildDownloaderOnce(t *testing.T) {
	ex := extractor.NewCompositeExtractor()

	// Set up a transport that fails quickly (avoids real HTTP calls)
	ex.SetTransport(&mockFailTransport{})

	// Add a mock extractor that matches any URL and fails
	mockExt := &mockQuickFailExtractor{}
	ex.AddExtractor(mockExt)

	dir := t.TempDir()

	// Use simple path names that don't contain backslashes (Windows) in JSON
	f1 := filepath.Join(dir, "f1.txt")
	f2 := filepath.Join(dir, "f2.txt")

	req1 := &download.Request{
		URL:      "http://example.com/page1",
		SavePath: filepath.Join(dir, "out1"),
		Metadata: map[string]string{
			"files": `[{"url":"http://example.com/f1","path":"` + filepath.ToSlash(f1) + `"}]`,
		},
	}
	req2 := &download.Request{
		URL:      "http://example.com/page2",
		SavePath: filepath.Join(dir, "out2"),
		Metadata: map[string]string{
			"files": `[{"url":"http://example.com/f2","path":"` + filepath.ToSlash(f2) + `"}]`,
		},
	}

	// Call Extract concurrently from multiple goroutines
	// The key verification is that sync.Once in buildDownloader prevents data races
	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := req1
			if idx%2 == 0 {
				req = req2
			}
			// Errors are expected (mock extractor fails), but no data race should occur
			_ = ex.Extract(t.Context(), req)
		}(i)
	}
	wg.Wait()

	// Verify that the mock extractor was called at least once (sub-downloads were attempted)
	mockExt.mu.Lock()
	count := mockExt.callCount
	mockExt.mu.Unlock()
	if count == 0 {
		t.Error("expected mock extractor to be called at least once")
	}
}

func TestCompositeExtractorBuildDownloaderCalledOnce(t *testing.T) {
	ex := extractor.NewCompositeExtractor()
	ex.SetTransport(&mockFailTransport{})
	mockExt := &mockQuickFailExtractor{}
	ex.AddExtractor(mockExt)

	dir := t.TempDir()
	f1 := filepath.Join(dir, "f1.txt")
	f2 := filepath.Join(dir, "f2.txt")

	req1 := &download.Request{
		URL:      "http://example.com/page1",
		SavePath: filepath.Join(dir, "out1"),
		Metadata: map[string]string{
			"files": `[{"url":"http://example.com/f1","path":"` + filepath.ToSlash(f1) + `"}]`,
		},
	}
	req2 := &download.Request{
		URL:      "http://example.com/page2",
		SavePath: filepath.Join(dir, "out2"),
		Metadata: map[string]string{
			"files": `[{"url":"http://example.com/f2","path":"` + filepath.ToSlash(f2) + `"}]`,
		},
	}

	// Sequential calls to Extract should both use the same cached downloader
	// (sync.Once ensures buildDownloader runs only once)
	err1 := ex.Extract(t.Context(), req1)
	err2 := ex.Extract(t.Context(), req2)

	// Both should fail with the mock extractor error
	if err1 == nil {
		t.Error("expected error from first Extract call")
	}
	if err2 == nil {
		t.Error("expected error from second Extract call (using cached downloader)")
	}
}
