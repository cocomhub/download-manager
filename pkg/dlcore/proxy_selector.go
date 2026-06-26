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

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/pkg/logutil"
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
		bandwidthSuffix:  config.DefaultBandwidthPath,
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

// getCachePath 返回 domain 对应的缓存文件路径。
// 如果未设置 cacheDir，使用系统缓存目录下的 dm-proxy-cache 子目录。
func (ps *DefaultProxySelector) getCachePath(domain string) string {
	cacheBase := ps.cacheDir
	if cacheBase == "" {
		dir, err := os.UserCacheDir()
		if err != nil {
			dir = os.TempDir()
		}
		cacheBase = filepath.Join(dir, "dm-proxy-cache")
	}
	return filepath.Join(cacheBase, domain)
}

// readCacheDecision 尝试从缓存文件读取决策。
// 返回 (true, "direct") 或 (true, "proxy") 表示有效缓存命中，(false, "") 表示未命中或已过期。
func (ps *DefaultProxySelector) readCacheDecision(cachePath string) (bool, string) {
	info, err := os.Stat(cachePath)
	if err != nil {
		return false, ""
	}
	ttl := ps.decisionCacheTTL
	if ttl <= 0 {
		ttl = 1
	}
	if time.Since(info.ModTime()) >= time.Duration(ttl)*time.Second {
		return false, ""
	}
	content, err := os.ReadFile(cachePath)
	if err != nil {
		return false, ""
	}
	return true, strings.TrimSpace(string(content))
}

// writeCacheDecision 将决策写入缓存文件（自动创建目录）。
func (ps *DefaultProxySelector) writeCacheDecision(cachePath, decision string) {
	_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
	_ = os.WriteFile(cachePath, []byte(decision), 0644)
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
	cachePath := ps.getCachePath(u.Host)

	if ok, decision := ps.readCacheDecision(cachePath); ok {
		if decision == "direct" {
			return "", nil
		}
		if decision == "proxy" {
			slog.Info("proxy cache hit, skipping direct check", "domain", u.Host)
			return ps.selectBestProxy(cachePath)
		}
	}

	if !ps.forceProxy && CheckDirect(targetURL, ps.forceProxy, ps.probeTimeout) {
		slog.Info("direct access is available", logutil.LogKeyURL, targetURL)
		ps.writeCacheDecision(cachePath, "direct")
		return "", nil
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
		ps.writeCacheDecision(cachePath, "proxy")
		return bestProxy, nil
	}
	return "", fmt.Errorf("no suitable proxy found")
}
