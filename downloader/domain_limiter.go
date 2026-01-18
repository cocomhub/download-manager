package downloader

import (
	"net/url"
	"sync"
)

type DomainLimiter struct {
	mu    sync.Mutex
	limit map[string]int
	cur   map[string]int
}

func NewDomainLimiter() *DomainLimiter {
	return &DomainLimiter{
		limit: make(map[string]int),
		cur:   make(map[string]int),
	}
}

func (d *DomainLimiter) Set(host string, max int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if max <= 0 {
		max = 1
	}
	d.limit[host] = max
}

func (d *DomainLimiter) Acquire(raw string) {
	u, err := url.Parse(raw)
	if err != nil {
		return
	}
	host := u.Host
	d.mu.Lock()
	max := d.limit[host]
	for max != 0 && d.cur[host] >= max {
		d.mu.Unlock()
		d.mu.Lock()
		max = d.limit[host]
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
	d.mu.Unlock()
}
