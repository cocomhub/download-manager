// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics 璁板綍鍗曚釜 Extractor/Transport 鐨勪笅杞界粺璁°€?type Metrics struct {
	Name            string
	TotalRequests   atomic.Int64
	TotalBytes      atomic.Int64
	SuccessCount    atomic.Int64
	FailureCount    atomic.Int64
	TotalDurationMs atomic.Int64
	LastRequestAt   atomic.Int64 // unix timestamp seconds
}

// MetricRegistry 绠＄悊鎵€鏈?Metrics 瀹炰緥锛屾寜鍚嶇О绱㈠紩銆?type MetricRegistry struct {
	mu      sync.Mutex
	metrics map[string]*Metrics
}

// NewMetricRegistry 鍒涘缓 MetricRegistry銆?func NewMetricRegistry() *MetricRegistry {
	return &MetricRegistry{
		metrics: make(map[string]*Metrics),
	}
}

// Get 杩斿洖鎴栧垱寤烘寚瀹氬悕绉扮殑 Metrics 瀹炰緥銆?func (r *MetricRegistry) Get(name string) *Metrics {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := r.metrics[name]; ok {
		return m
	}
	m := &Metrics{Name: name}
	r.metrics[name] = m
	return m
}

// Record 璁板綍涓€娆′笅杞界粨鏋溿€?func (r *MetricRegistry) Record(name string, bytes int64, duration time.Duration, success bool) {
	m := r.Get(name)
	m.TotalRequests.Add(1)
	m.TotalBytes.Add(bytes)
	m.TotalDurationMs.Add(duration.Milliseconds())
	m.LastRequestAt.Store(time.Now().Unix())
	if success {
		m.SuccessCount.Add(1)
	} else {
		m.FailureCount.Add(1)
	}
}

// Snapshot 杩斿洖鎵€鏈?metrics 鐨勫綋鍓嶅揩鐓э紙绾跨▼瀹夊叏锛夈€?// 杩斿洖 map[handler_name]map[field_name]value
func (r *MetricRegistry) Snapshot() map[string]map[string]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make(map[string]map[string]int64, len(r.metrics))
	for name, m := range r.metrics {
		result[name] = map[string]int64{
			"total_requests":    m.TotalRequests.Load(),
			"total_bytes":       m.TotalBytes.Load(),
			"success_count":     m.SuccessCount.Load(),
			"failure_count":     m.FailureCount.Load(),
			"total_duration_ms": m.TotalDurationMs.Load(),
			"last_request_at":   m.LastRequestAt.Load(),
		}
	}
	return result
}
