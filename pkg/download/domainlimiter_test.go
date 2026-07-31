// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestDomainLimiterConcurrent(t *testing.T) {
	t.Parallel()
	dl := NewDomainLimiter()
	dl.Set("example.com", 5)

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if err := dl.Acquire(t.Context(), "https://example.com/file"); err != nil {
				t.Errorf("Acquire should succeed: %v", err)
				return
			}
			time.Sleep(time.Millisecond) // simulate work
			dl.Release("https://example.com/file")
		})
	}
	wg.Wait()

	// After all goroutines complete, cur should be 0
	// 注意：同包测试直接访问私有字段是 Go 合法模式，外部包需通过导出方法访问。
	dl.mu.Lock()
	cur := dl.cur["example.com"]
	dl.mu.Unlock()
	if cur != 0 {
		t.Errorf("expected cur to be 0 after all releases, got %d", cur)
	}
}

func TestDomainLimiterSetWakeup(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 2)

	// Acquire 2 slots
	if err := dl.Acquire(t.Context(), "https://example.com/file1"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}
	if err := dl.Acquire(t.Context(), "https://example.com/file2"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}

	// 3rd acquire blocks
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	acquired3 := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		close(ready)
		err := dl.Acquire(ctx, "https://example.com/file3")
		acquired3 <- err
	}()
	<-ready

	// 注意：ready 通道仅通知 goroutine 已启动，但 Acquire 可能在 close 后仍未进入阻塞。
	// 2s 超时作为兜底，确保测试不会因竞态窗口而挂起。

	// Increase limit to 3, should wake up the waiter
	dl.Set("example.com", 3)

	select {
	case err := <-acquired3:
		if err != nil {
			t.Fatalf("Acquire should succeed after limit increase: %v", err)
		}
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("3rd acquire should have been unblocked after limit increase")
	}

	dl.Release("https://example.com/file1")
	dl.Release("https://example.com/file2")
	dl.Release("https://example.com/file3")
	cancel()
}

func TestDomainLimiterReleaseNoWaiters(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 1)

	// Acquire and release without any waiting goroutines
	if err := dl.Acquire(t.Context(), "https://example.com/file1"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}
	// Release should not panic or block
	dl.Release("https://example.com/file1")

	// Verify we can acquire again
	if err := dl.Acquire(t.Context(), "https://example.com/file2"); err != nil {
		t.Fatalf("Acquire should succeed after release: %v", err)
	}
	dl.Release("https://example.com/file2")
}

func TestDomainLimiterAcquireAfterCancel(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 1)

	// Acquire 1 slot
	if err := dl.Acquire(t.Context(), "https://example.com/file1"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}

	// 2nd acquire blocks, cancel it
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	acquired2 := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		close(ready)
		err := dl.Acquire(ctx, "https://example.com/file2")
		acquired2 <- err
	}()
	<-ready
	cancel()

	select {
	case err := <-acquired2:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Acquire should have returned after context cancellation")
	}

	// Release the first slot
	dl.Release("https://example.com/file1")

	// Now acquire again should work — the cancelled waiter was removed from the queue
	if err := dl.Acquire(t.Context(), "https://example.com/file3"); err != nil {
		t.Fatalf("Acquire should succeed after cancel+release: %v", err)
	}
	dl.Release("https://example.com/file3")
}

func TestDomainLimiterCancelThenRelease(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 1)

	if err := dl.Acquire(t.Context(), "https://example.com/file1"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}

	// 2nd acquire blocks, cancel it
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	acquired2 := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		close(ready)
		err := dl.Acquire(ctx, "https://example.com/file2")
		acquired2 <- err
	}()
	<-ready
	cancel()

	select {
	case err := <-acquired2:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Acquire should have returned after context cancellation")
	}

	// Release the first slot — the cancelled waiter was already removed from the queue
	// Release should not try to close an already-closed channel
	dl.Release("https://example.com/file1")
	// No panic, no deadlock: verified by reaching here
}
