// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"net/url"
	"sync"
)

// DomainLimiter 提供基于域名的并发连接数限制。
// 每个域名独立计数，超过限制的 acquire 会阻塞直到有释放信号。
type DomainLimiter struct {
	mu    sync.Mutex
	cond  *sync.Cond
	limit map[string]int
	cur   map[string]int
}

// NewDomainLimiter 创建并返回一个新的 DomainLimiter 实例。
func NewDomainLimiter() *DomainLimiter {
	d := &DomainLimiter{
		limit: make(map[string]int),
		cur:   make(map[string]int),
	}
	d.cond = sync.NewCond(&d.mu)
	return d
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
	d.cond.Broadcast()
}

// Acquire 尝试获取一个域的连接槽位。
// 如果当前连接数已达到限制，会阻塞直到有释放信号或超过限制变更。
// rawURL 可以是完整的 URL，内部会解析出 host。
func (d *DomainLimiter) Acquire(rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	host := u.Host
	d.mu.Lock()
	for max := d.limit[host]; max != 0 && d.cur[host] >= max; max = d.limit[host] {
		d.cond.Wait()
	}
	d.cur[host]++
	d.mu.Unlock()
}

// Release 释放一个域的连接槽位，唤醒等待者。
func (d *DomainLimiter) Release(rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	host := u.Host
	d.mu.Lock()
	if d.cur[host] > 0 {
		d.cur[host]--
	}
	d.cond.Broadcast()
	d.mu.Unlock()
}