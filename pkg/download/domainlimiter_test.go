// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"testing"
	"time"
)

func TestDomainLimiterSetAndAcquire(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 2)

	// Acquire 2 should succeed immediately
	dl.Acquire("https://example.com/file1")
	dl.Acquire("https://example.com/file2")

	// 3rd acquire should block, so we do it in a goroutine
	acquired3 := make(chan struct{}, 1)

	ready := make(chan struct{})
	go func() {
		close(ready) // signal that goroutine started
		dl.Acquire("https://example.com/file3")
		acquired3 <- struct{}{}
	}()
	<-ready // wait for goroutine to be scheduled

	// Release one, then the 3rd should get through
	dl.Release("https://example.com/file1")

	select {
	case <-acquired3:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("3rd acquire should have been unblocked after release")
	}

	// Clean up
	dl.Release("https://example.com/file2")
	dl.Release("https://example.com/file3")
}

func TestDomainLimiterReleaseUnknown(t *testing.T) {
	dl := NewDomainLimiter()

	// Should not panic
	dl.Release("https://unknown.example.com/file")
}

func TestDomainLimiterInvalidURL(t *testing.T) {
	dl := NewDomainLimiter()

	// Invalid URL should not panic
	dl.Acquire("://invalid-url")
	dl.Release("://invalid-url")
}

func TestDomainLimiterSetZero(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 0) // should clamp to 1

	dl.Acquire("https://example.com/file1")

	// 2nd acquire should block since limit is clamped to 1
	acquired2 := make(chan struct{}, 1)

	ready := make(chan struct{})
	go func() {
		close(ready) // signal that goroutine started
		dl.Acquire("https://example.com/file2")
		acquired2 <- struct{}{}
	}()
	<-ready // wait for goroutine to be scheduled

	// Release the first one
	dl.Release("https://example.com/file1")

	select {
	case <-acquired2:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("2nd acquire should have been unblocked after release")
	}

	dl.Release("https://example.com/file2")
}

func TestDomainLimiterNoLimit(t *testing.T) {
	dl := NewDomainLimiter()
	// No limit set for this domain - should allow any number
	dl.Acquire("https://unlimited.example.com/file1")
	dl.Acquire("https://unlimited.example.com/file2")
	dl.Acquire("https://unlimited.example.com/file3")
	// All should succeed immediately since no limit was set

	dl.Release("https://unlimited.example.com/file1")
	dl.Release("https://unlimited.example.com/file2")
	dl.Release("https://unlimited.example.com/file3")
}
