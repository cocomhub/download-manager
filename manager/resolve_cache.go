// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sync"
	"time"
)

// ResolveCache 内存 TTL 缓存，记录每个 URL 的 resolve 时间戳。
// 不持久化到 storage — resolved_at 作为内部状态。
type ResolveCache struct {
	mu      sync.RWMutex
	cache   map[string]time.Time
	ttl     time.Duration
	maxSize int
	done    chan struct{}
}

// NewResolveCache 创建 ResolveCache。
// ttl 过期时长，maxSize 超过此长度时触发同步清理。
func NewResolveCache(ttl time.Duration, maxSize int) *ResolveCache {
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

// MarkResolved 记录 key 在此时已 resolve。
func (c *ResolveCache) MarkResolved(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = time.Now()
	// 惰性清理：写入时检查是否需要清理
	if len(c.cache) > c.maxSize {
		c.evictLocked()
	}
}

// IsExpired 检查 key 是否需要重新 resolve。
func (c *ResolveCache) IsExpired(key string) bool {
	c.mu.RLock()
	t, ok := c.cache[key]
	c.mu.RUnlock()
	if !ok {
		return true
	}
	return time.Since(t) > c.ttl
}

// Invalidate 主动失效 key（resolve 失败时调用）。
func (c *ResolveCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, key)
}

// evictLocked 清理所有过期条目（调用者需持有写锁）。
func (c *ResolveCache) evictLocked() {
	now := time.Now()
	for k, t := range c.cache {
		if now.Sub(t) > c.ttl {
			delete(c.cache, k)
		}
	}
}

// Len 返回当前缓存条目数（仅用于测试）。
func (c *ResolveCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// Clear 清空所有条目。
func (c *ResolveCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.cache)
}
