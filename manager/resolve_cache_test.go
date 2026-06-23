// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sync"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/testutil/assert"
)

func TestResolveCache_MarkResolved(t *testing.T) {
	c := NewResolveCache(time.Hour, 100)
	c.MarkResolved("url1")
	if c.Len() != 1 {
		t.Errorf("expected 1 entry, got %d", c.Len())
	}
}

func TestResolveCache_IsExpired_Fresh(t *testing.T) {
	c := NewResolveCache(time.Hour, 100)
	c.MarkResolved("url1")
	if c.IsExpired("url1") {
		t.Error("expected fresh entry not expired")
	}
}

func TestResolveCache_IsExpired_NotExists(t *testing.T) {
	c := NewResolveCache(time.Hour, 100)
	if !c.IsExpired("unknown") {
		t.Error("expected unknown key to be expired")
	}
}

func TestResolveCache_IsExpired_AfterTTL(t *testing.T) {
	// 浣跨敤鏋佺煭 TTL 娴嬭瘯杩囨湡
	c := NewResolveCache(50*time.Millisecond, 100)
	c.MarkResolved("url1")
	assert.MustEventually(t, func() bool {
		return c.IsExpired("url1")
	}, 3*time.Second, 20*time.Millisecond, "wait for TTL expiry")
}

func TestResolveCache_Invalidate(t *testing.T) {
	c := NewResolveCache(time.Hour, 100)
	c.MarkResolved("url1")
	c.Invalidate("url1")
	if !c.IsExpired("url1") {
		t.Error("expected invalidated key to be expired")
	}
}

func TestResolveCache_EvictOnOverflow(t *testing.T) {
	// 璁剧疆 maxSize=2锛孴TL 鐭紝鏀?3 鏉″悗鏈€鏃╃殑搴旇娓呯悊
	c := NewResolveCache(50*time.Millisecond, 2)
	c.MarkResolved("url1")
	assert.MustEventually(t, func() bool {
		return c.IsExpired("url1")
	}, 3*time.Second, 20*time.Millisecond, "wait for url1 to expire")
	// url1 宸茶繃鏈燂紝浣嗚繕鏈Е鍙戞竻鐞?	c.MarkResolved("url2") // 瑙﹀彂 evict锛屾竻鐞?url1
	c.MarkResolved("url3")

	if c.Len() > 2 {
		t.Errorf("expected len <= 2 after evict, got %d", c.Len())
	}
}

func TestResolveCache_ConcurrentSafe(t *testing.T) {
	c := NewResolveCache(time.Hour, 1000)
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "url"
			c.MarkResolved(key)
			c.IsExpired(key)
			c.Invalidate(key)
		}(i)
	}
	wg.Wait()
}

func TestResolveCache_Clear(t *testing.T) {
	c := NewResolveCache(time.Hour, 100)
	c.MarkResolved("url1")
	c.MarkResolved("url2")
	c.Clear()
	if c.Len() != 0 {
		t.Errorf("expected 0 after clear, got %d", c.Len())
	}
}
