// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/cocomhub/download-manager/pkg/logutil"
)

// DomainLimiter 提供基于域名的并发连接数限制。
// 每个域名独立计数，超过限制的 acquire 会阻塞直到有释放信号。
// 支持 context 取消，取消时自动退出等待队列。
//
// 使用 atomic 管理 slot 计数，消除 channel 唤醒与 context 取消之间的竞态条件：
// - cur[host] 使用 atomic.Int64 管理活跃连接数
// - Acquire 成功时 atomic 递增 cur，失败时进入 waiter 队列
// - 被唤醒后不自动获得 slot，而是重新检查 cur < limit
type DomainLimiter struct {
	mu      sync.Mutex
	limit   map[string]int
	cur     map[string]*atomic.Int64
	waiters map[string][]chan struct{}
}

// NewDomainLimiter 创建并返回一个新的 DomainLimiter 实例。
func NewDomainLimiter() *DomainLimiter {
	return &DomainLimiter{
		limit:   make(map[string]int),
		cur:     make(map[string]*atomic.Int64),
		waiters: make(map[string][]chan struct{}),
	}
}

// getCur 获取或创建 host 对应的 atomic 计数器。
// 调用者必须持有 d.mu。
func (d *DomainLimiter) getCur(host string) *atomic.Int64 {
	c, ok := d.cur[host]
	if !ok {
		c = new(atomic.Int64)
		d.cur[host] = c
	}
	return c
}

// Set 设置指定主机的最大并发连接数。
// 如果 max <= 0，会被钳位为 1。
func (d *DomainLimiter) Set(host string, max int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if max <= 0 {
		max = 1
	}
	d.limit[host] = max

	waiters := d.waiters[host]
	if len(waiters) == 0 {
		return
	}

	c := d.getCur(host)
	available := max - int(c.Load())
	if available <= 0 {
		return
	}

	toWake := min(available, len(waiters))
	for i := range toWake {
		close(waiters[i])
	}
	d.waiters[host] = waiters[toWake:]
}

// Acquire 尝试获取一个域的连接槽位，支持 context 取消。
// 如果当前连接数已达到限制，会阻塞直到有释放信号或 context 被取消。
func (d *DomainLimiter) Acquire(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("domainlimiter: invalid URL: %w", err)
	}
	host := u.Host

	for {
		// 在拿锁之前先检查 context
		if err := ctx.Err(); err != nil {
			return err
		}

		d.mu.Lock()

		// 再次检查 context（拿锁期间可能已取消）
		if err := ctx.Err(); err != nil {
			d.mu.Unlock()
			return err
		}

		max := d.limit[host]
		c := d.getCur(host)
		cur := c.Load()

		if max == 0 || cur < int64(max) {
			c.Add(1)
			d.mu.Unlock()
			return nil
		}

		// 需要等待：创建通知通道加入等待队列
		ch := make(chan struct{})
		d.waiters[host] = append(d.waiters[host], ch)
		d.mu.Unlock()

		select {
		case <-ch:
			// 被 Set() 或 Release() 唤醒，重试循环
			continue
		case <-ctx.Done():
			// context 取消，从 waiter 队列移除自身
			d.mu.Lock()
			removed := d.removeWaiter(host, ch)
			if !removed {
				// 通道已被 Release/Set 移除——有 slot 被释放给了自己
				// 自己不消费，传给下一个 waiter
				if len(d.waiters[host]) > 0 {
					nextCh := d.waiters[host][0]
					d.waiters[host] = d.waiters[host][1:]
					close(nextCh)
				}
			}
			d.mu.Unlock()
			return ctx.Err()
		}
	}
}

// removeWaiter 从指定主机的等待队列中移除一个通道。
// 如果通道在队列中被找到并移除，返回 true。
// 如果通道不在队列中（已被 Release/Set 移除），返回 false。
// 调用者必须持有 d.mu。
func (d *DomainLimiter) removeWaiter(host string, ch chan struct{}) bool {
	waiters := d.waiters[host]
	for i, w := range waiters {
		if w == ch {
			d.waiters[host] = append(waiters[:i], waiters[i+1:]...)
			return true
		}
	}
	return false
}

// Release 释放一个域的连接槽位，唤醒一个等待者。
func (d *DomainLimiter) Release(rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		slog.Warn("DomainLimiter: failed to parse URL in Release, skipping", "url", rawURL, logutil.LogKeyError, err)
		return
	}
	host := u.Host

	d.mu.Lock()
	c := d.getCur(host)
	if c.Load() > 0 {
		c.Add(-1)
	}

	// 唤醒一个等待者
	if len(d.waiters[host]) > 0 {
		ch := d.waiters[host][0]
		d.waiters[host] = d.waiters[host][1:]
		close(ch)
	}
	d.mu.Unlock()
}

// Remove 删除指定主机的限制，并唤醒所有等待该主机的 waiter。
func (d *DomainLimiter) Remove(host string) {
	d.mu.Lock()
	delete(d.limit, host)
	if waiters := d.waiters[host]; len(waiters) > 0 {
		for _, ch := range waiters {
			close(ch)
		}
		delete(d.waiters, host)
	}
	d.mu.Unlock()
}
