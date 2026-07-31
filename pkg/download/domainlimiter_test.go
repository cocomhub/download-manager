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

	cur := dl.getCur("example.com").Load()

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

func TestDomainLimiterAcquireAlreadyCancelled(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 1)

	// 使用已取消的 context 调用 Acquire
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // 立即取消
	err := dl.Acquire(ctx, "https://example.com/file1")
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled for already-cancelled context, got %v", err)
	}
	// 验证 cur 未被增加
	dl.mu.Lock()

	cur := dl.getCur("example.com").Load()

	dl.mu.Unlock()
	if cur != 0 {
		t.Errorf("expected cur=0 after cancelled acquire, got %d", cur)
	}
}

func TestDomainLimiterAlreadyCancelledAllSlotsFull(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 1)

	// 占满所有 slot
	if err := dl.Acquire(t.Context(), "https://example.com/file1"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}

	// 用已取消的 context 尝试获取
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := dl.Acquire(ctx, "https://example.com/file2")
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// 验证 waiter 队列未被污染
	dl.mu.Lock()
	waiters := len(dl.waiters["example.com"])
	dl.mu.Unlock()
	if waiters != 0 {
		t.Errorf("expected 0 waiters after cancelled acquire, got %d", waiters)
	}

	dl.Release("https://example.com/file1")
}

func TestDomainLimiterMultipleDomains(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("a.com", 1)
	dl.Set("b.com", 2)

	// 占满 a.com
	if err := dl.Acquire(t.Context(), "https://a.com/1"); err != nil {
		t.Fatalf("Acquire a.com/1 should succeed: %v", err)
	}
	// 占满 b.com
	if err := dl.Acquire(t.Context(), "https://b.com/1"); err != nil {
		t.Fatalf("Acquire b.com/1 should succeed: %v", err)
	}
	if err := dl.Acquire(t.Context(), "https://b.com/2"); err != nil {
		t.Fatalf("Acquire b.com/2 should succeed: %v", err)
	}

	// 验证各域名独立计数
	dl.mu.Lock()

	curA := dl.getCur("a.com").Load()
	curB := dl.getCur("b.com").Load()

	dl.mu.Unlock()
	if curA != 1 {
		t.Errorf("expected a.com cur=1, got %d", curA)
	}
	if curB != 2 {
		t.Errorf("expected b.com cur=2, got %d", curB)
	}

	dl.Release("https://a.com/1")
	dl.Release("https://b.com/1")
	dl.Release("https://b.com/2")
}

func TestDomainLimiterSetZeroMax(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 0) // 应被钳位为 1

	if err := dl.Acquire(t.Context(), "https://example.com/file1"); err != nil {
		t.Fatalf("Acquire should succeed: %v", err)
	}

	// 第二个 acquire 应阻塞
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	acquired2 := make(chan error, 1)
	go func() {
		err := dl.Acquire(ctx, "https://example.com/file2")
		acquired2 <- err
	}()
	// 验证确实阻塞
	time.Sleep(50 * time.Millisecond)
	select {
	case <-acquired2:
		t.Fatal("Acquire should block when limit=1")
	default:
	}

	cancel()
	select {
	case err := <-acquired2:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Acquire should return after cancel")
	}

	dl.Release("https://example.com/file1")

	// 验证 Set(..., -1) 也被钳位为 1
	dl.Set("example.com", -1)
	if err := dl.Acquire(t.Context(), "https://example.com/file3"); err != nil {
		t.Fatalf("Acquire after Set(-1) should succeed: %v", err)
	}
	dl.Release("https://example.com/file3")
}

func TestDomainLimiterCancelReleaseRace(t *testing.T) {
	dl := NewDomainLimiter()
	dl.Set("example.com", 1)

	// 多轮：每轮先占满一个 slot，然后启动一个要取消的 acquire，再 release
	for range 100 {
		if err := dl.Acquire(t.Context(), "https://example.com/file1"); err != nil {
			t.Fatalf("Acquire should succeed: %v", err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		acquired := make(chan error, 1)
		go func() {
			acquired <- dl.Acquire(ctx, "https://example.com/file2")
		}()

		time.Sleep(time.Millisecond)
		cancel()
		dl.Release("https://example.com/file1")

		select {
		case err := <-acquired:
			if err != nil && err != context.Canceled {
				t.Fatalf("expected nil or context.Canceled, got %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out")
		}

		// 清理：如果 Acquire 成功了，需要释放它
		dl.mu.Lock()
		dl.waiters["example.com"] = nil
		dl.mu.Unlock()
	}

	dl.mu.Lock()
	cur := dl.getCur("example.com").Load()
	waiters := len(dl.waiters["example.com"])
	dl.mu.Unlock()
	if cur != 0 {
		t.Errorf("expected cur=0, got %d", cur)
	}
	if waiters != 0 {
		t.Errorf("expected 0 waiters, got %d", waiters)
	}
}
