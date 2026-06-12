// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package mockdl provides a mock implementation of core.Downloader for testing.
// It supports multiple behavior modes (always success, always fail, simulate
// progress, random failure, timeout, first-fail-then-success, pause-on-progress)
// and emits callbacks (OnStart, OnProgress, OnComplete, OnFail) for test assertions.
package mockdl

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// Mode defines the behavior of the MockDownloader.
type Mode string

const (
	// ModeAlwaysSuccess returns nil immediately.
	ModeAlwaysSuccess Mode = "always_success"
	// ModeAlwaysFail returns a fixed error immediately.
	ModeAlwaysFail Mode = "always_fail"
	// ModeSimulateProgress calls OnProgress from 0→100 then succeeds.
	ModeSimulateProgress Mode = "simulate_progress"
	// ModeRandomFail fails with probability FailRate.
	ModeRandomFail Mode = "random_fail"
	// ModeTimeout blocks until the context is cancelled.
	ModeTimeout Mode = "timeout"
	// ModeFirstFailThenSuccess fails the first Download call per URL,
	// then succeeds on subsequent calls. Coordinate tracking is in-memory.
	ModeFirstFailThenSuccess Mode = "first_fail_then_success"
	// ModePauseOnProgress pauses at InitialProgress and blocks until
	// the context is cancelled (simulates an in-flight cancellation).
	ModePauseOnProgress Mode = "pause_on_progress"
)

// ErrMockDownload is the default error returned by MockDownloader when failing.
var ErrMockDownload = errors.New("mock download failed")

// Option configures a MockDownloader.
type Option func(*MockDownloader)

// WithFailError sets the error returned on failure modes.
func WithFailError(err error) Option {
	return func(d *MockDownloader) { d.failErr = err }
}

// WithDelay sets a per-Download call delay.
func WithDelay(d time.Duration) Option {
	return func(md *MockDownloader) { md.delay = d }
}

// WithFailRate sets the failure probability for ModeRandomFail (0.0–1.0).
func WithFailRate(rate float64) Option {
	return func(md *MockDownloader) { md.failRate = rate }
}

// WithFailURLs marks specific URLs as always failing.
func WithFailURLs(urls ...string) Option {
	return func(md *MockDownloader) {
		if md.failURLs == nil {
			md.failURLs = make(map[string]bool)
		}
		for _, u := range urls {
			md.failURLs[u] = true
		}
	}
}

// WithTimeoutURLs marks specific URLs as timing out.
func WithTimeoutURLs(urls ...string) Option {
	return func(md *MockDownloader) {
		if md.timeoutURLs == nil {
			md.timeoutURLs = make(map[string]bool)
		}
		for _, u := range urls {
			md.timeoutURLs[u] = true
		}
	}
}

// WithDelayPerByte sets the simulated download speed.
func WithDelayPerByte(d time.Duration) Option {
	return func(md *MockDownloader) { md.delayPerByte = d }
}

// WithContext sets an external context that the MockDownloader uses
// for timeout/cancellation checks during simulation.
// If not set, context.Background() is used.
func WithContext(ctx context.Context) Option {
	return func(md *MockDownloader) { md.ctx = ctx }
}

// MockDownloader implements core.Downloader with configurable behaviors.
type MockDownloader struct {
	mu           sync.Mutex
	mode         Mode
	failErr      error
	delay        time.Duration
	failRate     float64
	failURLs     map[string]bool
	timeoutURLs  map[string]bool
	delayPerByte time.Duration

	// firstFailCount tracks the number of Download calls per URL
	// for ModeFirstFailThenSuccess.
	firstFailCount map[string]int

	// External context for timeout/cancellation. Falls back to background.
	ctx context.Context

	// Callbacks for test assertions.
	OnStart    func(url string)
	OnProgress func(url string, progress int)
	OnComplete func(url string)
	OnFail     func(url string, err error)
}

// New creates a MockDownloader with the given mode and options.
// Default behavior: ModeAlwaysSuccess, no delay.
func New(mode Mode, opts ...Option) *MockDownloader {
	d := &MockDownloader{
		mode:           mode,
		failErr:        ErrMockDownload,
		firstFailCount: make(map[string]int),
		ctx:            context.Background(),
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Name returns "mock".
func (d *MockDownloader) Name() string { return "mock" }

// Download implements core.Downloader.
func (d *MockDownloader) Download(obj *model.DownloadObject, _ map[string]string) error {
	if d.OnStart != nil {
		d.OnStart(obj.URL)
	}
	if d.delay > 0 {
		time.Sleep(d.delay)
	}

	// URL-specific overrides take priority.
	if d.failURLs[obj.URL] {
		return d.fail(obj)
	}
	if d.timeoutURLs[obj.URL] {
		return d.timeout(obj)
	}

	switch d.mode {
	case ModeAlwaysSuccess:
		return d.complete(obj)
	case ModeAlwaysFail:
		return d.fail(obj)
	case ModeSimulateProgress:
		return d.simulateProgress(obj)
	case ModeRandomFail:
		if rand.Float64() < d.failRate {
			return d.fail(obj)
		}
		return d.complete(obj)
	case ModeTimeout:
		return d.timeout(obj)
	case ModeFirstFailThenSuccess:
		return d.firstFail(obj)
	case ModePauseOnProgress:
		return d.pauseAtProgress(obj)
	default:
		return d.complete(obj)
	}
}

// SetContext implements core.DownloaderWithContext.
// If called with a non-nil context, the downloader will use it for
// cancellation checks during simulated progress and timeout modes.
func (d *MockDownloader) SetContext(ctx context.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if ctx != nil {
		d.ctx = ctx
	}
}

// getContext returns the current context under the mutex, defaulting to
// context.Background() if nil. This is used by internal helpers to safely
// read the context without racing against SetContext.
func (d *MockDownloader) getContext() context.Context {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ctx != nil {
		return d.ctx
	}
	return context.Background()
}
func (d *MockDownloader) complete(obj *model.DownloadObject) error {
	obj.SetProgress(100)
	obj.SetStatus(model.StatusCompleted)
	if d.OnComplete != nil {
		d.OnComplete(obj.URL)
	}
	return nil
}

func (d *MockDownloader) fail(obj *model.DownloadObject) error {
	obj.SetStatus(model.StatusFailed)
	if d.OnFail != nil {
		d.OnFail(obj.URL, d.failErr)
	}
	return fmt.Errorf("%w: %s", d.failErr, obj.URL)
}

func (d *MockDownloader) timeout(obj *model.DownloadObject) error {
	ctx := d.getContext()
	<-ctx.Done()
	obj.SetStatus(model.StatusFailed)
	err := context.Cause(ctx)
	if errors.Is(err, context.Canceled) {
		err = context.Canceled
	}
	if d.OnFail != nil {
		d.OnFail(obj.URL, err)
	}
	return err
}

// simulateProgress calls OnProgress from 0 to 100 in steps of 5,
// respecting context cancellation. It uses the object lock to safely
// access Extra fields during progress simulation.
func (d *MockDownloader) simulateProgress(obj *model.DownloadObject) error {
	fileSize := 1024 * 1024 // 1MB default for progress simulation

	// Read group_size under the object lock.
	obj.Lock()
	if obj.Extra != nil {
		if gs, ok := obj.Extra["group_size"]; ok {
			switch v := gs.(type) {
			case float64:
				if v > 0 {
					fileSize = int(v)
				}
			case int:
				if v > 0 {
					fileSize = v
				}
			case int64:
				if v > 0 {
					fileSize = int(v)
				}
			}
		}
	}
	// Copy lock-unrelated info before releasing.
	groupSize := fileSize
	if groupSize <= 0 {
		groupSize = 1024 * 1024
	}
	obj.Unlock()

	totalSteps := 20 // 5% steps
	for i := 0; i <= totalSteps; i++ {
		select {
		case <-d.getContext().Done():
			obj.SetStatus(model.StatusFailed)
			err := d.getContext().Err()
			if d.OnFail != nil {
				d.OnFail(obj.URL, err)
			}
			return err
		default:
		}

		pct := min(int(float64(i)/float64(totalSteps)*100), 100)
		obj.SetProgress(pct)

		if d.OnProgress != nil {
			d.OnProgress(obj.URL, pct)
		}

		// Simulate incremental progress timing.
		if d.delayPerByte > 0 {
			time.Sleep(d.delayPerByte * time.Duration(groupSize/totalSteps))
		} else {
			time.Sleep(10 * time.Millisecond)
		}
	}

	obj.SetStatus(model.StatusCompleted)
	if d.OnComplete != nil {
		d.OnComplete(obj.URL)
	}
	return nil
}

// firstFail fails the first Download call per URL, then succeeds.
func (d *MockDownloader) firstFail(obj *model.DownloadObject) error {
	d.mu.Lock()
	count := d.firstFailCount[obj.URL]
	d.firstFailCount[obj.URL] = count + 1
	d.mu.Unlock()

	if count == 0 {
		obj.SetStatus(model.StatusFailed)
		if d.OnFail != nil {
			d.OnFail(obj.URL, d.failErr)
		}
		return fmt.Errorf("%w (first attempt): %s", d.failErr, obj.URL)
	}

	return d.complete(obj)
}

// pauseAtProgress sets progress to the object's initial value and blocks
// until the context is cancelled.
func (d *MockDownloader) pauseAtProgress(obj *model.DownloadObject) error {
	prog := obj.GetProgress()
	if prog <= 0 {
		prog = 50
	}
	obj.SetProgress(prog)
	if d.OnProgress != nil {
		d.OnProgress(obj.URL, prog)
	}

	// Block until cancelled.
	<-d.getContext().Done()
	obj.SetStatus(model.StatusFailed)
	err := d.getContext().Err()
	if d.OnFail != nil {
		d.OnFail(obj.URL, err)
	}
	return err
}

// compile-time interface check.
var _ core.Downloader = (*MockDownloader)(nil)
var _ core.DownloaderWithContext = (*MockDownloader)(nil)
