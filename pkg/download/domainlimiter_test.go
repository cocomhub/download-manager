// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"testing"
	"time"
)

func TestDomainLimiterSetAndAcquire(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 2)

	// Acquire 2 should succeed immediately
	if err := dl.Acquire(t.Context(), "https://example.com/file1"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}
	if err := dl.Acquire(t.Context(), "https://example.com/file2"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}

	// 3rd acquire should block, so we do it in a goroutine
	ctx, cancel := context.WithCancel(t.Context())
	acquired3 := make(chan error, 1)

	ready := make(chan struct{})
	go func() {
		close(ready) // signal that goroutine started
		err := dl.Acquire(ctx, "https://example.com/file3")
		acquired3 <- err
	}()
	<-ready // wait for goroutine to be scheduled

	// Release one, then the 3rd should get through
	dl.Release("https://example.com/file1")

	select {
	case err := <-acquired3:
		if err != nil {
			t.Fatalf("Acquire should succeed: %v", err)
		}
	case <-time.After(2 * time.Second):
		cancel() // cleanup goroutine
		t.Fatal("3rd acquire should have been unblocked after release")
	}

	// Clean up
	dl.Release("https://example.com/file2")
	dl.Release("https://example.com/file3")
	cancel()
}

func TestDomainLimiterReleaseUnknown(t *testing.T) {
	dl := NewDomainLimiter()

	// Should not panic
	dl.Release("https://unknown.example.com/file")
}

func TestDomainLimiterInvalidURL(t *testing.T) {
	dl := NewDomainLimiter()

	// Invalid URL should return an error
	err := dl.Acquire(t.Context(), "://invalid-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
	dl.Release("://invalid-url")
}

func TestDomainLimiterSetZero(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 0) // should clamp to 1

	if err := dl.Acquire(t.Context(), "https://example.com/file1"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}

	// 2nd acquire should block since limit is clamped to 1
	ctx, cancel := context.WithCancel(t.Context())
	acquired2 := make(chan error, 1)

	ready := make(chan struct{})
	go func() {
		close(ready) // signal that goroutine started
		err := dl.Acquire(ctx, "https://example.com/file2")
		acquired2 <- err
	}()
	<-ready // wait for goroutine to be scheduled

	// Release the first one
	dl.Release("https://example.com/file1")

	select {
	case err := <-acquired2:
		if err != nil {
			t.Fatalf("Acquire should succeed after release: %v", err)
		}
	case <-time.After(2 * time.Second):
		cancel() // cleanup goroutine
		t.Fatal("2nd acquire should have been unblocked after release")
	}

	dl.Release("https://example.com/file2")
	cancel()
}

func TestDomainLimiterNoLimit(t *testing.T) {
	dl := NewDomainLimiter()
	// No limit set for this domain - should allow any number
	if err := dl.Acquire(t.Context(), "https://unlimited.example.com/file1"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}
	if err := dl.Acquire(t.Context(), "https://unlimited.example.com/file2"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}
	if err := dl.Acquire(t.Context(), "https://unlimited.example.com/file3"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}
	// All should succeed immediately since no limit was set

	dl.Release("https://unlimited.example.com/file1")
	dl.Release("https://unlimited.example.com/file2")
	dl.Release("https://unlimited.example.com/file3")
}

func TestDomainLimiterContextCancel(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 1)

	if err := dl.Acquire(t.Context(), "https://example.com/file1"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}

	// 2nd acquire should block; we cancel its context
	ctx, cancel := context.WithCancel(t.Context())
	acquired2 := make(chan error, 1)

	ready := make(chan struct{})
	go func() {
		close(ready)
		err := dl.Acquire(ctx, "https://example.com/file2")
		acquired2 <- err
	}()
	<-ready

	// Cancel the context while the goroutine is waiting
	cancel()

	select {
	case err := <-acquired2:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Acquire should have returned after context cancellation")
	}

	dl.Release("https://example.com/file1")
}

func TestDomainLimiterContextCancelMulti(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 1)

	if err := dl.Acquire(t.Context(), "https://example.com/file1"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}

	// Two waiters, cancel one and release the other
	ctx1, cancel1 := context.WithCancel(t.Context())
	ctx2, cancel2 := context.WithCancel(t.Context())

	acquired1 := make(chan error, 1)
	acquired2 := make(chan error, 1)
	ready1 := make(chan struct{})
	ready2 := make(chan struct{})

	go func() {
		close(ready1)
		err := dl.Acquire(ctx1, "https://example.com/file2")
		acquired1 <- err
	}()
	go func() {
		close(ready2)
		err := dl.Acquire(ctx2, "https://example.com/file3")
		acquired2 <- err
	}()
	<-ready1
	<-ready2

	// Cancel the first waiter
	cancel1()

	select {
	case err := <-acquired1:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		cancel2()
		t.Fatal("first Acquire should have returned after context cancellation")
	}

	// Release the first slot, the second waiter should get through
	dl.Release("https://example.com/file1")

	select {
	case err := <-acquired2:
		if err != nil {
			t.Fatalf("Acquire should succeed after release: %v", err)
		}
	case <-time.After(2 * time.Second):
		cancel2()
		t.Fatal("second Acquire should have been unblocked after release")
	}

	dl.Release("https://example.com/file3")
	cancel2()
}
