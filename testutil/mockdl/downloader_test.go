// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mockdl

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/testutil/assert"
)

func TestMockDownloader_Name(t *testing.T) {
	d := New(ModeAlwaysSuccess)
	if got := d.Name(); got != "mock" {
		t.Errorf("Name() = %q, want %q", got, "mock")
	}
}

func TestMockDownloader_AlwaysSuccess(t *testing.T) {
	obj := testObject("http://example.com/file1.bin")
	d := New(ModeAlwaysSuccess)

	err := d.Download(obj, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj.GetStatus() != model.StatusCompleted {
		t.Errorf("status = %q, want %q", obj.GetStatus(), model.StatusCompleted)
	}
	if obj.GetProgress() != 100 {
		t.Errorf("progress = %d, want 100", obj.GetProgress())
	}
}

func TestMockDownloader_AlwaysSuccess_FiresCallbacks(t *testing.T) {
	var started, completed atomic.Bool
	obj := testObject("http://example.com/cb.bin")
	d := New(ModeAlwaysSuccess,
		WithFailError(errors.New("should-not-fire")),
	)
	d.OnStart = func(url string) {
		if url != obj.URL {
			t.Errorf("OnStart url = %q, want %q", url, obj.URL)
		}
		started.Store(true)
	}
	d.OnComplete = func(url string) {
		if url != obj.URL {
			t.Errorf("OnComplete url = %q, want %q", url, obj.URL)
		}
		completed.Store(true)
	}
	d.OnFail = func(url string, err error) {
		t.Errorf("OnFail should not fire, got url=%q err=%v", url, err)
	}

	_ = d.Download(obj, nil)
	if !started.Load() {
		t.Error("OnStart was not called")
	}
	if !completed.Load() {
		t.Error("OnComplete was not called")
	}
}

var errCustom = errors.New("custom download error")

func TestMockDownloader_AlwaysFail(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		// check 接收 Download 返回的 err 和传入的 obj，做具体断言
		check func(t *testing.T, err error, obj *model.DownloadObject)
	}{
		{
			name: "basic",
			check: func(t *testing.T, err error, obj *model.DownloadObject) {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if obj.GetStatus() != model.StatusFailed {
					t.Errorf("status = %q, want %q", obj.GetStatus(), model.StatusFailed)
				}
			},
		},
		{
			name: "custom error",
			opts: []Option{WithFailError(errCustom)},
			check: func(t *testing.T, err error, obj *model.DownloadObject) {
				if !errors.Is(err, errCustom) {
					t.Errorf("error = %v, want %v", err, errCustom)
				}
			},
		},
		{
			name: "fires callbacks",
			check: func(t *testing.T, err error, obj *model.DownloadObject) {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := testObject("http://example.com/fail-" + tc.name + ".bin")
			d := New(ModeAlwaysFail, tc.opts...)

			var started, failed atomic.Bool
			d.OnStart = func(url string) { started.Store(true) }
			d.OnFail = func(url string, err error) {
				if url != obj.URL {
					t.Errorf("OnFail url = %q, want %q", url, obj.URL)
				}
				failed.Store(true)
			}
			d.OnComplete = func(url string) {
				t.Error("OnComplete should not fire on failure")
			}

			err := d.Download(obj, nil)
			tc.check(t, err, obj)

			if !started.Load() {
				t.Error("OnStart was not called")
			}
			if !failed.Load() {
				t.Error("OnFail was not called")
			}
		})
	}
}

func TestMockDownloader_SimulateProgress(t *testing.T) {
	var progressCalls []int
	obj := testObject("http://example.com/prog.bin")
	obj.Extra = map[string]any{"group_size": 1024}

	d := New(ModeSimulateProgress, WithDelay(0))
	d.OnProgress = func(url string, pct int) {
		progressCalls = append(progressCalls, pct)
	}

	err := d.Download(obj, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj.GetStatus() != model.StatusCompleted {
		t.Errorf("status = %q, want %q", obj.GetStatus(), model.StatusCompleted)
	}
	if obj.GetProgress() != 100 {
		t.Errorf("progress = %d, want 100", obj.GetProgress())
	}
	if len(progressCalls) == 0 {
		t.Error("OnProgress was not called")
	} else if progressCalls[len(progressCalls)-1] != 100 {
		t.Errorf("last progress = %d, want 100", progressCalls[len(progressCalls)-1])
	}
}

func TestMockDownloader_RandomFail(t *testing.T) {
	tests := []struct {
		name      string
		failRate  float64
		wantError bool
	}{
		{name: "always fail (rate=1.0)", failRate: 1.0, wantError: true},
		{name: "always succeed (rate=0.0)", failRate: 0.0, wantError: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := New(ModeRandomFail, WithFailRate(tc.failRate))
			obj := testObject("http://example.com/rand-" + tc.name + ".bin")

			err := d.Download(obj, nil)
			if tc.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if obj.GetStatus() != model.StatusCompleted {
					t.Errorf("status = %q, want %q", obj.GetStatus(), model.StatusCompleted)
				}
			}
		})
	}
}

func TestMockDownloader_Timeout(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // immediately cancelled, so timeout returns right away

	obj := testObject("http://example.com/timeout.bin")
	d := New(ModeTimeout, WithContext(ctx))

	err := d.Download(obj, nil)
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
	if obj.GetStatus() != model.StatusFailed {
		t.Errorf("status = %q, want %q", obj.GetStatus(), model.StatusFailed)
	}
}

func TestMockDownloader_FirstFailThenSuccess(t *testing.T) {
	url := "http://example.com/retry.bin"
	d := New(ModeFirstFailThenSuccess)

	// First call: should fail.
	obj1 := testObject(url)
	err1 := d.Download(obj1, nil)
	if err1 == nil {
		t.Fatal("expected error on first attempt, got nil")
	}
	if obj1.GetStatus() != model.StatusFailed {
		t.Errorf("after first attempt status = %q, want %q", obj1.GetStatus(), model.StatusFailed)
	}

	// Second call (same URL): should succeed.
	obj2 := testObject(url)
	err2 := d.Download(obj2, nil)
	if err2 != nil {
		t.Fatalf("unexpected error on second attempt: %v", err2)
	}
	if obj2.GetStatus() != model.StatusCompleted {
		t.Errorf("after second attempt status = %q, want %q", obj2.GetStatus(), model.StatusCompleted)
	}
}

func TestMockDownloader_PauseOnProgress_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	obj := testObject("http://example.com/pause.bin")
	obj.SetProgress(30)

	d := New(ModePauseOnProgress, WithContext(ctx))

	// Start the download in a goroutine; it will block at 30%.
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Download(obj, nil)
	}()

	// Wait for the download to reach the pause point (progress >= 30).
	assert.MustEventually(t, func() bool {
		return obj.GetProgress() >= 30
	}, time.Second, 10*time.Millisecond, "wait for download to reach progress 30")

	// Cancel to unblock.
	cancel()

	select {
	case err := <-errCh:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for cancelled download to return")
	}
}

func TestMockDownloader_FailURLs(t *testing.T) {
	failURL := "http://example.com/always-fail.bin"
	okURL := "http://example.com/always-ok.bin"

	d := New(ModeAlwaysSuccess, WithFailURLs(failURL))

	// URL in fail list should fail even in always_success mode.
	objFail := testObject(failURL)
	err := d.Download(objFail, nil)
	if err == nil {
		t.Fatal("expected error for URL in FailURLs, got nil")
	}

	// Other URL should succeed.
	objOK := testObject(okURL)
	err = d.Download(objOK, nil)
	if err != nil {
		t.Fatalf("unexpected error for non-fail URL: %v", err)
	}
}

func TestMockDownloader_TimeoutURLs(t *testing.T) {
	timeoutURL := "http://example.com/slow.bin"
	okURL := "http://example.com/fast.bin"
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // immediate cancellation, so timeout URL returns right away

	d := New(ModeAlwaysSuccess, WithTimeoutURLs(timeoutURL), WithContext(ctx))

	// URL in timeout list should block and return cancelled error.
	objID := testObject(timeoutURL)
	err := d.Download(objID, nil)
	if err == nil {
		t.Fatal("expected error for URL in TimeoutURLs, got nil")
	}

	// Other URL should succeed (always_success mode).
	objOK := testObject(okURL)
	err = d.Download(objOK, nil)
	if err != nil {
		t.Fatalf("unexpected error for non-timeout URL: %v", err)
	}
}

func TestMockDownloader_Delay(t *testing.T) {
	obj := testObject("http://example.com/delay.bin")
	d := New(ModeAlwaysSuccess, WithDelay(50*time.Millisecond))

	start := time.Now()
	_ = d.Download(obj, nil)
	elapsed := time.Since(start)

	if elapsed < 40*time.Millisecond {
		t.Errorf("expected delay ~50ms, got %v", elapsed)
	}
}

func TestMockDownloader_SetContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	d := New(ModeSimulateProgress)
	d.SetContext(ctx)

	errCh := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		close(ready)
		obj := testObject("http://example.com/setctx.bin")
		errCh <- d.Download(obj, nil)
	}()
	<-ready

	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected error after context cancellation, got nil")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for download to return after SetContext cancel")
	}
}

func testObject(url string) *model.DownloadObject {
	return &model.DownloadObject{
		URL:    url,
		Status: model.StatusPending,
	}
}
