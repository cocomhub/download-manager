// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"bytes"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- ComposeProgress tests ---

func TestComposeProgressNilReturnsNil(t *testing.T) {
	cb := ComposeProgress(nil, nil, nil)
	if cb != nil {
		t.Error("expected nil when all callbacks are nil")
	}
}

func TestComposeProgressSingleReturnsSame(t *testing.T) {
	var called bool
	original := func(float64, int64, int64) { called = true }
	cb := ComposeProgress(original)
	cb(100, 1000, 1000)
	if !called {
		t.Error("expected original callback to be called")
	}
}

func TestComposeProgressAllCalled(t *testing.T) {
	var (
		mu    sync.Mutex
		order []int
	)
	cb1 := func(float64, int64, int64) {
		mu.Lock()
		order = append(order, 1)
		mu.Unlock()
	}
	cb2 := func(float64, int64, int64) {
		mu.Lock()
		order = append(order, 2)
		mu.Unlock()
	}
	cb3 := func(float64, int64, int64) {
		mu.Lock()
		order = append(order, 3)
		mu.Unlock()
	}

	combined := ComposeProgress(cb1, cb2, cb3)
	combined(50, 500, 1000)

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("expected order [1,2,3], got %v", order)
	}
}

func TestComposeProgressNilFiltered(t *testing.T) {
	var count int32
	cb := func(float64, int64, int64) {
		atomic.AddInt32(&count, 1)
	}
	combined := ComposeProgress(cb, nil, cb)
	combined(50, 500, 1000)
	if n := atomic.LoadInt32(&count); n != 2 {
		t.Errorf("expected 2 calls, got %d", n)
	}
}

func TestComposeProgressArgumentsPreserved(t *testing.T) {
	var prog float64
	var down, tot int64
	combined := ComposeProgress(
		func(p float64, d, t int64) {
			prog = p
			down = d
			tot = t
		},
	)
	combined(42.5, 425, 1000)
	if prog != 42.5 {
		t.Errorf("expected progress 42.5, got %f", prog)
	}
	if down != 425 {
		t.Errorf("expected downloaded 425, got %d", down)
	}
	if tot != 1000 {
		t.Errorf("expected total 1000, got %d", tot)
	}
}

// --- NewProgressLogCallback tests ---

func TestFirstCallAlwaysWrites(t *testing.T) {
	var buf bytes.Buffer
	cb := NewProgressLogCallback(WithLogWriter(&buf))
	cb(0, 0, 1000)
	if !strings.Contains(buf.String(), "Progress:") {
		t.Errorf("expected first call to write, got: %s", buf.String())
	}
}

func TestSkipsBelowMinStep(t *testing.T) {
	var buf bytes.Buffer
	cb := NewProgressLogCallback(
		WithLogWriter(&buf),
		WithMinPercentStep(5.0),
		WithMaxInterval(time.Hour),
	)
	// 1st call: always writes
	cb(0, 0, 1000)
	// 2nd call: delta=2.0 < 5.0, should NOT write
	cb(2.0, 20, 1000)
	// 3rd call: delta=1.0 < 5.0, should NOT write
	cb(3.0, 30, 1000)

	lines := strings.Count(strings.TrimSpace(buf.String()), "\n") + 1
	if buf.Len() > 0 && strings.TrimSpace(buf.String()) == "" {
		lines = 0
	}
	// Should only have 1 line (from first call)
	if lines != 1 {
		t.Errorf("expected 1 line (first call only), got %d lines:\n%s", lines, buf.String())
	}
}

func TestWritesOnMaxInterval(t *testing.T) {
	var buf bytes.Buffer
	cb := NewProgressLogCallback(
		WithLogWriter(&buf),
		WithMinPercentStep(50.0),
		WithMaxInterval(50*time.Millisecond),
	)
	// 1st call: always writes
	cb(0, 0, 1000)
	// sleep past maxInterval
	<-time.After(60 * time.Millisecond)
	// 2nd call: delta=0.5 < 50, but interval exceeded => should write
	cb(0.5, 5, 1000)

	lines := strings.Count(strings.TrimSpace(buf.String()), "\n") + 1
	if lines != 2 {
		t.Errorf("expected 2 lines (first + interval-triggered), got %d:\n%s", lines, buf.String())
	}
}

func TestDefaultFormatterIncludesHundredPercent(t *testing.T) {
	var buf bytes.Buffer
	cb := NewProgressLogCallback(
		WithLogWriter(&buf),
		WithMinPercentStep(0),
	)
	cb(100, 1000, 1000)
	output := buf.String()
	if !strings.Contains(output, "100.000%") {
		t.Errorf("expected output to contain '100.000%%', got:\n%s", output)
	}
	if !strings.Contains(output, "expected time:") {
		t.Errorf("expected output to contain 'expected time:', got:\n%s", output)
	}
}

func TestNilWriterNoPanic(t *testing.T) {
	cb := NewProgressLogCallback(WithLogWriter(nil))
	// Should not panic
	cb(50, 500, 1000)
	cb(100, 1000, 1000)
}
