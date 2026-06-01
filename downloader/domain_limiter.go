// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"net/url"
	"sync"
)

type DomainLimiter struct {
	mu    sync.Mutex
	cond  *sync.Cond
	limit map[string]int
	cur   map[string]int
}

func NewDomainLimiter() *DomainLimiter {
	d := &DomainLimiter{
		limit: make(map[string]int),
		cur:   make(map[string]int),
	}
	d.cond = sync.NewCond(&d.mu)
	return d
}

func (d *DomainLimiter) Set(host string, max int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if max <= 0 {
		max = 1
	}
	d.limit[host] = max
	d.cond.Broadcast()
}

func (d *DomainLimiter) Acquire(raw string) {
	u, err := url.Parse(raw)
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

func (d *DomainLimiter) Release(raw string) {
	u, err := url.Parse(raw)
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
