// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"net/url"
	"sync"
)

// DomainLimiter 鎻愪緵鍩轰簬鍩熷悕鐨勫苟鍙戣繛鎺ユ暟闄愬埗銆?// 姣忎釜鍩熷悕鐙珛璁℃暟锛岃秴杩囬檺鍒剁殑 acquire 浼氶樆濉炵洿鍒版湁閲婃斁淇″彿銆?type DomainLimiter struct {
	mu    sync.Mutex
	cond  *sync.Cond
	limit map[string]int
	cur   map[string]int
}

// NewDomainLimiter 鍒涘缓骞惰繑鍥炰竴涓柊鐨?DomainLimiter 瀹炰緥銆?func NewDomainLimiter() *DomainLimiter {
	d := &DomainLimiter{
		limit: make(map[string]int),
		cur:   make(map[string]int),
	}
	d.cond = sync.NewCond(&d.mu)
	return d
}

// Set 璁剧疆鎸囧畾涓绘満鐨勬渶澶у苟鍙戣繛鎺ユ暟銆?// 濡傛灉 max <= 0锛屼細琚挸浣嶄负 1銆?func (d *DomainLimiter) Set(host string, max int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if max <= 0 {
		max = 1
	}
	d.limit[host] = max
	d.cond.Broadcast()
}

// Acquire 灏濊瘯鑾峰彇涓€涓煙鐨勮繛鎺ユЫ浣嶃€?// 濡傛灉褰撳墠杩炴帴鏁板凡杈惧埌闄愬埗锛屼細闃诲鐩村埌鏈夐噴鏀句俊鍙锋垨瓒呰繃闄愬埗鍙樻洿銆?// rawURL 鍙互鏄畬鏁寸殑 URL锛屽唴閮ㄤ細瑙ｆ瀽鍑?host銆?func (d *DomainLimiter) Acquire(rawURL string) {
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

// Release 閲婃斁涓€涓煙鐨勮繛鎺ユЫ浣嶏紝鍞ら啋绛夊緟鑰呫€?func (d *DomainLimiter) Release(rawURL string) {
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
