// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"

	"github.com/cocomhub/download-manager/pkg/logutil"
)

// DomainLimiter 提供基于域名的并发连接数限制。
// 每个域名独立计数，超过限制的 acquire 会阻塞直到有释放信号。
// 支持 context 取消，取消时自动退出等待队列。
type DomainLimiter struct {
	mu      sync.Mutex
	limit   map[string]int
	cur     map[string]int
	waiters map[string][]chan struct{}
}

// NewDomainLimiter 创建并返回一个新的 DomainLimiter 实例。
func NewDomainLimiter() *DomainLimiter {
	return &DomainLimiter{
		limit:   make(map[string]int),
		cur:     make(map[string]int),
		waiters: make(map[string][]chan struct{}),
	}
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
	available := max - d.cur[host]
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
// rawURL 可以是完整的 URL，内部会解析出 host。
func (d *DomainLimiter) Acquire(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("domainlimiter: invalid URL: %w", err)
	}
	host := u.Host

	d.mu.Lock()
	for {
		// 在检查条件之前先检查 context 是否已取消
		if err := ctx.Err(); err != nil {
			d.mu.Unlock()
			return err
		}

		max := d.limit[host]
		if max == 0 || d.cur[host] < max {
			d.cur[host]++
			d.mu.Unlock()
			return nil
		}

		// 需要等待：创建通知通道加入等待队列
		ch := make(chan struct{})
		d.waiters[host] = append(d.waiters[host], ch)
		d.mu.Unlock()

		select {
		case <-ch:
			// 被唤醒，重新获取锁
			d.mu.Lock()
			// 被唤醒后检查 context 是否已被取消。
			// 如果已被取消，不能使用这个 slot，需要传递给下一个等待者。
			if err := ctx.Err(); err != nil {
				// 传递 slot 给下一个等待者（如果有）
				// Release 已减 cur，或 Set 已增 limit，slot 已释放
				d.passSlot(host)
				d.mu.Unlock()
				return err
			}
			// 继续 for 循环重新检查条件
		case <-ctx.Done():
			// context 被取消，从等待队列中移除
			d.mu.Lock()
			d.removeWaiter(host, ch)
			d.mu.Unlock()
			return ctx.Err()
		}
	}
}

// passSlot 将当前可用的 slot 传递给下一个等待者（如果有）。
// 调用者必须持有 d.mu。
func (d *DomainLimiter) passSlot(host string) {
	if len(d.waiters[host]) > 0 {
		nextCh := d.waiters[host][0]
		d.waiters[host] = d.waiters[host][1:]
		close(nextCh)
	}
}

// removeWaiter 从指定主机的等待队列中移除一个通道。
// 如果通道在队列中被找到并移除，返回 true。如果通道不在队列中（已被 Release/Set 移除），返回 false。
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
	host := rawURL
	if err != nil {
		slog.Warn("DomainLimiter: failed to parse URL in Release, using raw URL as key", "url", rawURL, logutil.LogKeyError, err)
	} else {
		host = u.Host
	}
	d.mu.Lock()
	if d.cur[host] > 0 {
		d.cur[host]--
	}
	// 唤醒一个等待者
	if len(d.waiters[host]) > 0 {
		ch := d.waiters[host][0]
		d.waiters[host] = d.waiters[host][1:]
		close(ch)
	}
	d.mu.Unlock()
}
