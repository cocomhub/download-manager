// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/config"
)

// StaticProxySelector 是静态代理列表的选择器实现。
// 它使用文件缓存 + 直连探测 + 带宽评分来选择最佳代理。
type StaticProxySelector struct {
	proxies          []string
	forceProxy       bool
	cacheDir         string
	decisionCacheTTL int // seconds
	probeTimeout     int // seconds
	bandwidthSuffix  string
}

// NewStaticProxySelector 创建基于静态代理列表的选择器。
// 默认值：
//   - 决策缓存 TTL：1 秒
//   - 探测超时：3 秒
//   - 带宽路径后缀："/bandwidth"
func NewStaticProxySelector(proxies []string) *StaticProxySelector {
	return &StaticProxySelector{
		proxies:          proxies,
		decisionCacheTTL: 1,
		probeTimeout:     3,
		bandwidthSuffix:  config.DefaultBandwidthPath,
	}
}

// WithForceProxy 设置是否强制使用代理（跳过直连探测）。
func (s *StaticProxySelector) WithForceProxy(v bool) *StaticProxySelector {
	s.forceProxy = v
	return s
}

// WithCache 设置代理决策缓存目录和 TTL（天数）。
func (s *StaticProxySelector) WithCache(dir string, ttl int) *StaticProxySelector {
	s.cacheDir = dir
	s.decisionCacheTTL = ttl
	return s
}

// WithProbe 设置直连探测超时（秒）。
func (s *StaticProxySelector) WithProbe(timeout int) *StaticProxySelector {
	s.probeTimeout = timeout
	return s
}

// cachePathForDomain 返回指定域名的缓存文件路径。
func (s *StaticProxySelector) cachePathForDomain(domain string) string {
	cacheBase := s.cacheDir
	if cacheBase == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			cacheDir = os.TempDir()
		}
		cacheBase = filepath.Join(cacheDir, "dm-proxy-cache")
	}
	return filepath.Join(cacheBase, domain)
}

// readCachedDecision 读取并验证缓存中的代理决策。
// 返回决策值（"direct"/"proxy"）和是否命中有效缓存。
func (s *StaticProxySelector) readCachedDecision(cachePath string) (string, bool) {
	info, err := os.Stat(cachePath)
	if err != nil {
		return "", false
	}
	ttl := s.decisionCacheTTL
	if ttl <= 0 {
		ttl = 1
	}
	if time.Since(info.ModTime()) >= time.Duration(ttl)*time.Second {
		return "", false
	}
	content, err := os.ReadFile(cachePath)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(content)), true
}

// writeCacheDecision 将代理决策写入缓存文件。
func (s *StaticProxySelector) writeCacheDecision(cachePath string, decision string) {
	_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
	_ = os.WriteFile(cachePath, []byte(decision), 0644)
}

// Select 实现 ProxySelector 接口。
// 返回空字符串表示直连（不使用代理）。
func (s *StaticProxySelector) Select(ctx context.Context, targetURL string, hint *DownloadHint) (string, error) {
	if len(s.proxies) == 0 {
		return "", nil
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		return "", err
	}

	cachePath := s.cachePathForDomain(u.Host)

	// 检查缓存
	if decision, ok := s.readCachedDecision(cachePath); ok {
		if decision == "direct" {
			return "", nil
		}
		return s.selectBestProxy(ctx, cachePath)
	}

	// 直连探测
	if !s.forceProxy && checkDirect(ctx, targetURL, s.probeTimeout) {
		s.writeCacheDecision(cachePath, "direct")
		return "", nil
	}

	return s.selectBestProxy(ctx, cachePath)
}

// selectBestProxy 执行带宽扫描，选出最佳代理并写入缓存。
func (s *StaticProxySelector) selectBestProxy(ctx context.Context, cachePath string) (string, error) {
	bestProxy := ""
	minBandwidth := 999999.0
	for _, p := range s.proxies {
		bw := getProxyBandwidth(ctx, p, s.bandwidthSuffix, s.probeTimeout)
		if bw < minBandwidth {
			minBandwidth = bw
			bestProxy = p
		}
	}
	if bestProxy != "" {
		s.writeCacheDecision(cachePath, "proxy")
		return bestProxy, nil
	}
	return "", fmt.Errorf("no suitable proxy found")
}

// checkDirect 检测是否可直接访问目标 URL。返回 true 表示可直连。
func checkDirect(ctx context.Context, targetURL string, timeoutSecs int) bool {
	if timeoutSecs <= 0 {
		timeoutSecs = 3
	}
	client := &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second}
	hreq, err := http.NewRequestWithContext(ctx, "HEAD", targetURL, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(hreq)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// getProxyBandwidth 查询代理的带宽值（数值越小越好），失败时返回 999999。
func getProxyBandwidth(ctx context.Context, proxyURL, suffix string, timeoutSecs int) float64 {
	if strings.TrimSpace(suffix) == "" {
		suffix = "/bandwidth"
	}
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	target := fmt.Sprintf("%s%s", strings.TrimRight(proxyURL, "/"), suffix)
	if timeoutSecs <= 0 {
		timeoutSecs = 3
	}
	client := &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second}
	hreq, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return 999999
	}
	resp, err := client.Do(hreq)
	if err != nil {
		return 999999
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 999999
	}
	val, err := strconv.ParseFloat(strings.TrimSpace(string(body)), 64)
	if err != nil {
		return 999999
	}
	return val
}
