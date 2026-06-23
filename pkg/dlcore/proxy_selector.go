// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dlcore

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProxySelector 定义代理选择策略。
// Select 返回最佳代理 URL（空字符串表示直连），以及可能的错误。
type ProxySelector interface {
	Select(targetURL string) (proxyURL string, err error)
}

// DefaultProxySelector 是默认的代理选择器实现，使用文件缓存 + 直连探测 + 带宽评分。
type DefaultProxySelector struct {
	proxies          []string
	forceProxy       bool
	cacheDir         string
	decisionCacheTTL int // seconds
	probeTimeout     int // seconds
	bandwidthSuffix  string
}

// NewProxySelector 创建默认代理选择器，使用给定的代理列表。
// 其余参数使用合理默认值：
//   - 决策缓存 TTL：1 秒
//   - 探测超时：3 秒
//   - 带宽路径后缀："/bandwidth"
func NewProxySelector(proxies []string) *DefaultProxySelector {
	return &DefaultProxySelector{
		proxies:          proxies,
		decisionCacheTTL: 1,
		probeTimeout:     3,
		bandwidthSuffix:  "/bandwidth",
	}
}

// WithForceProxy 设置是否强制使用代理（跳过直连探测）。
func (ps *DefaultProxySelector) WithForceProxy(v bool) *DefaultProxySelector {
	ps.forceProxy = v
	return ps
}

// WithCache 设置缓存目录和决策 TTL（秒）。
// ttlSecs 为 0 时使用默认值。
func (ps *DefaultProxySelector) WithCache(dir string, ttlSecs int) *DefaultProxySelector {
	ps.cacheDir = dir
	if ttlSecs > 0 {
		ps.decisionCacheTTL = ttlSecs
	}
	return ps
}

// WithProbe 设置直连探测超时（秒）。timeoutSecs 为 0 时使用默认值。
func (ps *DefaultProxySelector) WithProbe(timeoutSecs int) *DefaultProxySelector {
	if timeoutSecs > 0 {
		ps.probeTimeout = timeoutSecs
	}
	return ps
}

// WithBandwidthSuffix 设置代理带宽探测路径后缀。默认为 "/bandwidth"。
func (ps *DefaultProxySelector) WithBandwidthSuffix(s string) *DefaultProxySelector {
	if s != "" {
		ps.bandwidthSuffix = s
	}
	return ps
}

// Select 执行代理选择流程：
//  1. 无代理配置时直接返回直连
//  2. 检查文件缓存（domain 粒度的直连/代理决策）
//  3. 缓存命中且为直连 → 直连
//  4. 缓存命中但为代理 → 跳过直连探测，直接带宽扫描
//  5. 未命中缓存 → 直连探测 + 带宽扫描
//  6. 写入缓存
func (ps *DefaultProxySelector) Select(targetURL string) (string, error) {
	if len(ps.proxies) == 0 {
		return "", nil
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		return "", err
	}
	domain := u.Host

	cacheBase := ps.cacheDir
	if cacheBase == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			cacheDir = os.TempDir()
		}
		cacheBase = filepath.Join(cacheDir, "dm-proxy-cache")
	}
	cachePath := filepath.Join(cacheBase, domain)

	// 检查缓存
	if info, err := os.Stat(cachePath); err == nil {
		ttl := ps.decisionCacheTTL
		if ttl <= 0 {
			ttl = 1
		}
		if time.Since(info.ModTime()) < time.Duration(ttl)*time.Second {
			content, _ := os.ReadFile(cachePath)
			s := strings.TrimSpace(string(content))
			if s == "direct" {
				return "", nil
			}
			if s == "proxy" {
				slog.Info("proxy cache hit, skipping direct check", "domain", domain)
				return ps.selectBestProxy(cachePath)
			}
		}
	}

	// 直连探测
	if !ps.forceProxy {
		if CheckDirect(targetURL, ps.forceProxy, ps.probeTimeout) {
			slog.Info("direct access is available", "url", targetURL)
			_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
			_ = os.WriteFile(cachePath, []byte("direct"), 0644)
			return "", nil
		}
	}

	return ps.selectBestProxy(cachePath)
}

// selectBestProxy 执行带宽扫描，选出最佳代理并写入缓存。
func (ps *DefaultProxySelector) selectBestProxy(cachePath string) (string, error) {
	bestProxy := ""
	minBandwidth := 999999.0
	for _, p := range ps.proxies {
		bw := GetProxyBandwidth(p, ps.bandwidthSuffix, ps.probeTimeout)
		if bw < minBandwidth {
			minBandwidth = bw
			bestProxy = p
		}
	}
	if bestProxy != "" {
		slog.Info("best proxy found", "proxy", bestProxy, "bandwidth", minBandwidth)
		_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
		_ = os.WriteFile(cachePath, []byte("proxy"), 0644)
		return bestProxy, nil
	}
	return "", fmt.Errorf("no suitable proxy found")
}
