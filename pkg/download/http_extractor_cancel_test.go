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
	"github.com/cocomhub/download-manager/testutil/assert"
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

	// 使用轮询等待 cancel func 注册并确认 Cancel 成功
	// 直接访问 e.cancels 因包外不可见，通过轮询检测
	if canceller, ok := any(ext).(download.Canceller); ok {
		assert.MustEventually(t, func() bool {
			err := canceller.Cancel(ts.URL)
			return err == nil
		}, time.Second, 50*time.Millisecond, "cancel should succeed")
		t.Logf("Cancel returned: %v", canceller.Cancel(ts.URL))
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

	canceller, ok := any(ext).(download.Canceller)
	if !ok {
		t.Fatal("HTTPExtractor does not implement Canceller interface")
	}
	err := canceller.Cancel("http://nonexistent.url/file.bin")
	if err != nil {
		t.Errorf("Cancel on non-existent URL should return nil, got: %v", err)
	}
	// 第二次调用也应返回 nil（幂等性）
	err = canceller.Cancel("http://nonexistent.url/file.bin")
	if err != nil {
		t.Errorf("Cancel on non-existent URL (second call) should return nil, got: %v", err)
	}
}

func TestHTTPExtractorTimeout(t *testing.T) {
	ts := newSlowServer(t, 10*time.Second)
	defer ts.Close()

	dir := t.TempDir()
	dest := dir + "/timeout_test.bin"

	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	err := ext.Extract(ctx, &download.Request{
		URL:      ts.URL,
		SavePath: dest,
	})
	if err == nil {
		t.Error("expected timeout error, got nil")
	} else {
		t.Logf("timeout resulted in error (expected): %v", err)
	}
}
