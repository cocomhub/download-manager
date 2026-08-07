// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
)

// retryCancelServer 返回 500 使提取器进入重试循环。
// firstRequestCh 在收到第一个 HTTP 请求时关闭，用于同步。
func newRetryServer(t *testing.T, firstRequestCh chan struct{}) *httptest.Server {
	t.Helper()
	var once sync.Once
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(firstRequestCh) })
		w.WriteHeader(http.StatusInternalServerError)
	}))
}

func TestHTTPExtractorRetryCancel(t *testing.T) {
	firstRequestCh := make(chan struct{})
	retrySleepCh := make(chan struct{}) // 测试钩子关闭此通道，表示已进入 retry sleep 阶段
	ts := newRetryServer(t, firstRequestCh)
	defer ts.Close()

	dir := t.TempDir()
	dest := dir + "/retry_cancel.bin"

	ext := download.NewHTTPExtractor()
	ext.SetTransport(download.NewStdlibTransport())
	// 设置测试钩子：在进入 retry sleep 前关闭 retrySleepCh，用于同步
	var once sync.Once
	ext.TestHookRetrySleep = func() {
		once.Do(func() { close(retrySleepCh) })
	}

	// Create a context we can cancel during retry wait
	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error, 1)
	go func() {
		errCh <- ext.Extract(ctx, &download.Request{
			URL:      ts.URL,
			SavePath: dest,
		})
	}()

	// 等待第一个 HTTP 请求被服务器接收（500 立即返回）
	<-firstRequestCh
	// 等待确认已进入 retry sleep 阶段（钩子已在 time.Sleep 前触发）
	<-retrySleepCh

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
