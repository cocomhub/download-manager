// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
)

// retryCancelServer returns 500 repeatedly so the extractor enters retry loop.
func newRetryServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
}

func TestHTTPExtractorRetryCancel(t *testing.T) {
	ts := newRetryServer(t)
	defer ts.Close()

	dir := t.TempDir()
	dest := dir + "/retry_cancel.bin"

	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())

	// Create a context we can cancel during retry wait
	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error, 1)
	startCh := make(chan struct{})
	go func() {
		close(startCh)
		errCh <- ext.Extract(ctx, &download.Request{
			URL:      ts.URL,
			SavePath: dest,
		})
	}()

	// Wait for the goroutine to start
	<-startCh
	// Give the first attempt time to fail and enter the retry sleep
	time.Sleep(500 * time.Millisecond)

	// Cancel the context while the retry is waiting
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected cancel error, got nil")
		} else {
			t.Logf("Retry canceled with error (expected): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("retry did not cancel within 5s — context was not respected during retry sleep")
	}
}
