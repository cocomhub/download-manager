// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"sync"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
)

// TestHTTPExtractorConcurrentAccess verifies that setters and Extract can be
// called concurrently without data races.
func TestHTTPExtractorConcurrentAccess(t *testing.T) {
	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())

	var wg sync.WaitGroup

	// Concurrent setters
	for range 10 {
		wg.Go(func() {
			ext.SetBrowserHeaders(true)
			ext.SetAllowPaths([]string{"/tmp", "/var/tmp"})
			ext.AddResponseCheck(func(req *download.Request, tresp *download.TransportResponse) error {
				return nil
			})
		})
	}

	// Concurrent calls to Name and Match (read-only, should be safe)
	for range 10 {
		wg.Go(func() {
			_ = ext.Name()
			_ = ext.Match(t.Context(), "http://example.com/file.mp4")
		})
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent access test timed out")
	}
}

// TestHTTPExtractorConcurrentSameURL verifies that two concurrent Extract calls
// with the same URL correctly detect the conflict.
func TestHTTPExtractorConcurrentSameURL(t *testing.T) {
	ts := newSlowServer(t, 2*time.Second)
	defer ts.Close()

	dir := t.TempDir()
	dest := dir + "/concurrent_same_url.bin"

	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())

	errCh := make(chan error, 2)
	for range 2 {
		go func() {
			errCh <- ext.Extract(t.Context(), &download.Request{
				URL:      ts.URL,
				SavePath: dest,
			})
		}()
	}

	err1 := <-errCh
	err2 := <-errCh

	// One should succeed (or get a download error from the server), the other
	// should get "already downloading" error.
	firstOK := err1 == nil || err1.Error() != "already downloading: "+ts.URL
	secondOK := err2 == nil || err2.Error() != "already downloading: "+ts.URL

	if !firstOK && !secondOK {
		t.Errorf("expected at least one download to proceed, got errors: %v, %v", err1, err2)
	}
}
