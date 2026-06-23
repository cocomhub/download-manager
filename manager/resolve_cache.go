// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sync"
	"time"
)

// ResolveCache 鍐呭瓨 TTL 缂撳瓨锛岃褰曟瘡涓?URL 鐨?resolve 鏃堕棿鎴炽€?// 涓嶆寔涔呭寲鍒?storage 鈥?resolved_at 浣滀负鍐呴儴鐘舵€併€?type ResolveCache struct {
	mu      sync.RWMutex
	cache   map[string]time.Time
	ttl     time.Duration
	maxSize int
	done    chan struct{}
}

// NewResolveCache 鍒涘缓 ResolveCache銆?// ttl 杩囨湡鏃堕暱锛宮axSize 瓒呰繃姝ら暱搴︽椂瑙﹀彂鍚屾娓呯悊銆?func NewResolveCache(ttl time.Duration, maxSize int) *ResolveCache {
	if ttl <= 0 {
		ttl = time.Hour
	}
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &ResolveCache{
		cache:   make(map[string]time.Time),
		ttl:     ttl,
		maxSize: maxSize,
		done:    make(chan struct{}),
	}
}

// MarkResolved 璁板綍 key 鍦ㄦ鏃跺凡 resolve銆?func (c *ResolveCache) MarkResolved(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = time.Now()
	// 鎯版€ф竻鐞嗭細鍐欏叆鏃舵鏌ユ槸鍚﹂渶瑕佹竻鐞?	if len(c.cache) > c.maxSize {
		c.evictLocked()
	}
}

// IsExpired 妫€鏌?key 鏄惁闇€瑕侀噸鏂?resolve銆?func (c *ResolveCache) IsExpired(key string) bool {
	c.mu.RLock()
	t, ok := c.cache[key]
	c.mu.RUnlock()
	if !ok {
		return true
	}
	return time.Since(t) > c.ttl
}

// Invalidate 涓诲姩澶辨晥 key锛坮esolve 澶辫触鏃惰皟鐢級銆?func (c *ResolveCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, key)
}

// evictLocked 娓呯悊鎵€鏈夎繃鏈熸潯鐩紙璋冪敤鑰呴渶鎸佹湁鍐欓攣锛夈€?func (c *ResolveCache) evictLocked() {
	now := time.Now()
	for k, t := range c.cache {
		if now.Sub(t) > c.ttl {
			delete(c.cache, k)
		}
	}
}

// Len 杩斿洖褰撳墠缂撳瓨鏉＄洰鏁帮紙浠呯敤浜庢祴璇曪級銆?func (c *ResolveCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// Clear 娓呯┖鎵€鏈夋潯鐩€?func (c *ResolveCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.cache)
}
