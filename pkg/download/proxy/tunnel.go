// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

// TunnelInstance 描述一个 sproxy 隧道实例。
type TunnelInstance struct {
	ServerURL string // e.g. "http://192.168.1.100:18083"
	TunnelKey string // 64 hex chars, AES-256-GCM key
}

// TunnelProxySelector 在多个 sproxy 实例中选择最优节点。
// 通过健康检查和带宽探测评估节点，结果缓存在内存中。
type TunnelProxySelector struct {
	instances      []TunnelInstance
	mu             sync.RWMutex
	probeTimeout   time.Duration
	probeBytes     int64
	cacheTTL       time.Duration
	bandwidthCache map[string]cachedBandwidth
	cacheMu        sync.RWMutex
}

type cachedBandwidth struct {
	bandwidth float64
	checkedAt time.Time
}

// NewTunnelProxySelector 创建 TunnelProxySelector。
func NewTunnelProxySelector(opts ...TunnelOption) *TunnelProxySelector {
	s := &TunnelProxySelector{
		probeTimeout:   5 * time.Second,
		probeBytes:     512 * 1024, // 512KB probe
		cacheTTL:       30 * time.Second,
		bandwidthCache: make(map[string]cachedBandwidth),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// TunnelOption 配置 TunnelProxySelector。
type TunnelOption func(*TunnelProxySelector)

// WithTunnelInstance 添加一个 sproxy 隧道实例。
func WithTunnelInstance(serverURL, tunnelKey string) TunnelOption {
	return func(s *TunnelProxySelector) {
		s.instances = append(s.instances, TunnelInstance{
			ServerURL: strings.TrimRight(serverURL, "/"),
			TunnelKey: tunnelKey,
		})
	}
}

// WithProbeSize 设置带宽探测的字节数。
func WithProbeSize(bytes int64) TunnelOption {
	return func(s *TunnelProxySelector) { s.probeBytes = bytes }
}

// WithProbeTimeout 设置探测超时时间。
func WithProbeTimeout(d time.Duration) TunnelOption {
	return func(s *TunnelProxySelector) { s.probeTimeout = d }
}

// WithCacheTTL 设置缓存有效期。
func WithCacheTTL(d time.Duration) TunnelOption {
	return func(s *TunnelProxySelector) { s.cacheTTL = d }
}

// getCached 返回缓存的带宽值，过期时返回 false。
func (s *TunnelProxySelector) getCached(serverURL string) (float64, bool) {
	s.cacheMu.RLock()
	c, ok := s.bandwidthCache[serverURL]
	s.cacheMu.RUnlock()
	if !ok {
		return 0, false
	}
	if time.Since(c.checkedAt) > s.cacheTTL {
		return 0, false
	}
	return c.bandwidth, true
}

// setCache 写入带宽缓存。
func (s *TunnelProxySelector) setCache(serverURL string, bw float64) {
	s.cacheMu.Lock()
	s.bandwidthCache[serverURL] = cachedBandwidth{
		bandwidth: bw,
		checkedAt: time.Now(),
	}
	s.cacheMu.Unlock()
}

// Select 实现 download.ProxySelector 接口，返回最优 sproxy 实例的 URL。
func (s *TunnelProxySelector) Select(ctx context.Context, targetURL string, hint *download.DownloadHint) (string, error) {
	s.mu.RLock()
	instances := make([]TunnelInstance, len(s.instances))
	copy(instances, s.instances)
	s.mu.RUnlock()

	if len(instances) == 0 {
		return "", nil
	}

	// 检查每个实例的健康状态
	type instanceResult struct {
		serverURL string
		bandwidth float64
	}

	var results []instanceResult
	for _, inst := range instances {
		// 健康检查
		healthURL := inst.ServerURL + "/healthz"
		if err := download.CheckHealth(ctx, healthURL, s.probeTimeout); err != nil {
			slog.Debug("sproxy instance unhealthy", logutil.LogKeyURL, inst.ServerURL, logutil.LogKeyError, err)
			continue
		}

		// 缓存命中
		if bw, ok := s.getCached(inst.ServerURL); ok {
			results = append(results, instanceResult{serverURL: inst.ServerURL, bandwidth: bw})
			continue
		}

		// 带宽探测（使用 /bandwidth 端点获取足够大的响应）
		bw, err := download.CheckBandwidth(ctx, inst.ServerURL+"/bandwidth", s.probeBytes, s.probeTimeout)
		if err != nil {
			slog.Debug("sproxy bandwidth probe failed", logutil.LogKeyURL, inst.ServerURL, logutil.LogKeyError, err)
			results = append(results, instanceResult{serverURL: inst.ServerURL, bandwidth: 0})
		} else {
			s.setCache(inst.ServerURL, bw)
			results = append(results, instanceResult{serverURL: inst.ServerURL, bandwidth: bw})
		}
	}

	if len(results) == 0 {
		// 没有可用实例，返回空（直连）
		return "", nil
	}

	// 按带宽降序排列，选带宽最高的
	sort.Slice(results, func(i, j int) bool {
		return results[i].bandwidth > results[j].bandwidth
	})

	best := results[0]
	slog.Info("Selected sproxy instance", logutil.LogKeyURL, best.serverURL, "bandwidth", best.bandwidth)
	return best.serverURL, nil
}
