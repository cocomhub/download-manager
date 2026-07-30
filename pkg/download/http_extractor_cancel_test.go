// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
)

// newSlowServer creates an httptest.Server that responds slowly,
// allowing us to test mid-download cancellation.
func newSlowServer(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		// Write a small amount of data then delay to keep the connection open
		_, _ = w.Write([]byte{0})
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		// Block until the client disconnects or the delay expires
		select {
		case <-r.Context().Done():
			// Client disconnected
		case <-time.After(delay):
		}
	}))
}

func TestHTTPExtractorCancel(t *testing.T) {
	ts := newSlowServer(t, 10*time.Second)
	defer ts.Close()

	dir := t.TempDir()
	dest := dir + "/cancel_test.bin"

	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())

	// 使用 ready channel 在调用 Extract 前发出信号
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		close(ready)
		errCh <- ext.Extract(t.Context(), &download.Request{
			URL:      ts.URL,
			SavePath: dest,
		})
	}()

	<-ready

	// 给 Extract 一点时间注册 cancel func 并开始 HTTP 请求
	// 因无法从外部访问 e.cancels，使用短延迟替代轮询
	time.Sleep(50 * time.Millisecond)

	// Cancel via the URL-based cancel method
	if canceller, ok := any(ext).(download.Canceller); ok {
		err := canceller.Cancel(ts.URL)
		t.Logf("Cancel returned: %v", err)
	} else {
		t.Fatal("HTTPExtractor does not implement Canceller interface")
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected cancel error, got nil")
		} else {
			t.Logf("cancel resulted in error (expected): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("download did not cancel within 5s")
	}
}

func TestHTTPExtractorCancelNotFound(t *testing.T) {
	ext := download.NewHTTPExtractor()

	if canceller, ok := any(ext).(download.Canceller); ok {
		err := canceller.Cancel("http://nonexistent.url/file.bin")
		t.Logf("Cancel returned for non-existent URL: %v", err)
	} else {
		t.Fatal("HTTPExtractor does not implement Canceller interface")
	}
}
