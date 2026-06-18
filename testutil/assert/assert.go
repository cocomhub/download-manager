// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package assert provides test assertion helpers, including Eventually/MustEventually
// for polling-based async condition checking.
package assert

import (
	"testing"
	"time"
)

// Eventually polls fn until it returns true or timeout elapses.
// It returns true if fn returned true within the timeout, false otherwise.
// If t is provided, it calls t.Helper() for better error reporting,
// but does NOT fail the test — the caller decides what to do on timeout.
func Eventually(t testing.TB, fn func() bool, timeout, interval time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(interval)
	}
	return false
}

// MustEventually is like Eventually but calls t.Fatal on timeout.
// It requires a format string and optional args to explain what timed out.
func MustEventually(t testing.TB, fn func() bool, timeout, interval time.Duration, msgAndArgs ...any) {
	t.Helper()
	if !Eventually(t, fn, timeout, interval) {
		if len(msgAndArgs) > 0 {
			msg, ok := msgAndArgs[0].(string)
			if ok {
				t.Fatalf("assert.MustEventually timed out after %v: "+msg, append([]any{timeout}, msgAndArgs[1:]...)...)
				return
			}
		}
		t.Fatalf("assert.MustEventually timed out after %v", timeout)
	}
}
