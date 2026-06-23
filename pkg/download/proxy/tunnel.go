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

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/pkg/download"
)

// TunnelInstance 鎻忚堪涓€涓?sproxy 闅ч亾瀹炰緥銆?type TunnelInstance struct {
	ServerURL string // e.g. "http://192.168.1.100:18083"
	TunnelKey string // 64 hex chars, AES-256-GCM key
}

// TunnelProxySelector 鍦ㄥ涓?sproxy 瀹炰緥涓€夋嫨鏈€浼樿妭鐐广€?// 閫氳繃鍋ュ悍妫€鏌ュ拰甯﹀鎺㈡祴璇勪及鑺傜偣锛岀粨鏋滅紦瀛樺湪鍐呭瓨涓€?type TunnelProxySelector struct {
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

// NewTunnelProxySelector 鍒涘缓 TunnelProxySelector銆?func NewTunnelProxySelector(opts ...TunnelOption) *TunnelProxySelector {
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

// TunnelOption 閰嶇疆 TunnelProxySelector銆?type TunnelOption func(*TunnelProxySelector)

// WithTunnelInstance 娣诲姞涓€涓?sproxy 闅ч亾瀹炰緥銆?func WithTunnelInstance(serverURL, tunnelKey string) TunnelOption {
	return func(s *TunnelProxySelector) {
		s.instances = append(s.instances, TunnelInstance{
			ServerURL: strings.TrimRight(serverURL, "/"),
			TunnelKey: tunnelKey,
		})
	}
}

// WithProbeSize 璁剧疆甯﹀鎺㈡祴鐨勫瓧鑺傛暟銆?func WithProbeSize(bytes int64) TunnelOption {
	return func(s *TunnelProxySelector) { s.probeBytes = bytes }
}

// WithProbeTimeout 璁剧疆鎺㈡祴瓒呮椂鏃堕棿銆?func WithProbeTimeout(d time.Duration) TunnelOption {
	return func(s *TunnelProxySelector) { s.probeTimeout = d }
}

// WithCacheTTL 璁剧疆缂撳瓨鏈夋晥鏈熴€?func WithCacheTTL(d time.Duration) TunnelOption {
	return func(s *TunnelProxySelector) { s.cacheTTL = d }
}

// getCached 杩斿洖缂撳瓨鐨勫甫瀹藉€硷紝杩囨湡鏃惰繑鍥?false銆?func (s *TunnelProxySelector) getCached(serverURL string) (float64, bool) {
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

// setCache 鍐欏叆甯﹀缂撳瓨銆?func (s *TunnelProxySelector) setCache(serverURL string, bw float64) {
	s.cacheMu.Lock()
	s.bandwidthCache[serverURL] = cachedBandwidth{
		bandwidth: bw,
		checkedAt: time.Now(),
	}
	s.cacheMu.Unlock()
}

// Select 瀹炵幇 download.ProxySelector 鎺ュ彛锛岃繑鍥炴渶浼?sproxy 瀹炰緥鐨?URL銆?func (s *TunnelProxySelector) Select(ctx context.Context, targetURL string, hint *download.DownloadHint) (string, error) {
	s.mu.RLock()
	instances := make([]TunnelInstance, len(s.instances))
	copy(instances, s.instances)
	s.mu.RUnlock()

	if len(instances) == 0 {
		return "", nil
	}

	// 妫€鏌ユ瘡涓疄渚嬬殑鍋ュ悍鐘舵€?	type instanceResult struct {
		serverURL string
		bandwidth float64
	}

	var results []instanceResult
	for _, inst := range instances {
		// 鍋ュ悍妫€鏌?		healthURL := inst.ServerURL + "/healthz"
		if err := download.CheckHealth(ctx, healthURL, s.probeTimeout); err != nil {
			slog.Debug("sproxy instance unhealthy", "url", inst.ServerURL, "error", err)
			continue
		}

		// 缂撳瓨鍛戒腑
		if bw, ok := s.getCached(inst.ServerURL); ok {
			results = append(results, instanceResult{serverURL: inst.ServerURL, bandwidth: bw})
			continue
		}

		// 甯﹀鎺㈡祴锛堜娇鐢?/bandwidth 绔偣鑾峰彇瓒冲澶х殑鍝嶅簲锛?		bw, err := download.CheckBandwidth(ctx, inst.ServerURL+config.DefaultBandwidthPath, s.probeBytes, s.probeTimeout)
		if err != nil {
			slog.Debug("sproxy bandwidth probe failed", "url", inst.ServerURL, "error", err)
			results = append(results, instanceResult{serverURL: inst.ServerURL, bandwidth: 0})
		} else {
			s.setCache(inst.ServerURL, bw)
			results = append(results, instanceResult{serverURL: inst.ServerURL, bandwidth: bw})
		}
	}

	if len(results) == 0 {
		// 娌℃湁鍙敤瀹炰緥锛岃繑鍥炵┖锛堢洿杩烇級
		return "", nil
	}

	// 鎸夊甫瀹介檷搴忔帓鍒楋紝閫夊甫瀹芥渶楂樼殑
	sort.Slice(results, func(i, j int) bool {
		return results[i].bandwidth > results[j].bandwidth
	})

	best := results[0]
	slog.Info("Selected sproxy instance", "url", best.serverURL, "bandwidth", best.bandwidth)
	return best.serverURL, nil
}
