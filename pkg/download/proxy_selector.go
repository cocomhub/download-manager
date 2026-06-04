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
		bandwidthSuffix:  "/bandwidth",
	}
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
	domain := u.Host

	cacheBase := s.cacheDir
	if cacheBase == "" {
		cacheBase = filepath.Join(os.TempDir(), "download_proxy_cache")
	}
	cachePath := filepath.Join(cacheBase, domain)

	// 检查缓存
	if info, err := os.Stat(cachePath); err == nil {
		ttl := s.decisionCacheTTL
		if ttl <= 0 {
			ttl = 1
		}
		if time.Since(info.ModTime()) < time.Duration(ttl)*time.Second {
			content, _ := os.ReadFile(cachePath)
			contentStr := strings.TrimSpace(string(content))
			if contentStr == "direct" {
				return "", nil
			}
			if contentStr == "proxy" {
				return s.selectBestProxy(cachePath)
			}
		}
	}

	// 直连探测
	if !s.forceProxy {
		if checkDirect(targetURL, s.probeTimeout) {
			_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
			_ = os.WriteFile(cachePath, []byte("direct"), 0644)
			return "", nil
		}
	}

	return s.selectBestProxy(cachePath)
}

// selectBestProxy 执行带宽扫描，选出最佳代理并写入缓存。
func (s *StaticProxySelector) selectBestProxy(cachePath string) (string, error) {
	bestProxy := ""
	minBandwidth := 999999.0
	for _, p := range s.proxies {
		bw := getProxyBandwidth(p, s.bandwidthSuffix, s.probeTimeout)
		if bw < minBandwidth {
			minBandwidth = bw
			bestProxy = p
		}
	}
	if bestProxy != "" {
		_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
		_ = os.WriteFile(cachePath, []byte("proxy"), 0644)
		return bestProxy, nil
	}
	return "", fmt.Errorf("no suitable proxy found")
}

// checkDirect 检测是否可直接访问目标 URL。返回 true 表示可直连。
func checkDirect(targetURL string, timeoutSecs int) bool {
	if timeoutSecs <= 0 {
		timeoutSecs = 3
	}
	client := &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second}
	hreq, err := http.NewRequest("HEAD", targetURL, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(hreq)
	if err != nil || resp.StatusCode != 200 {
		return false
	}
	resp.Body.Close()
	return true
}

// getProxyBandwidth 查询代理的带宽值（数值越小越好），失败时返回 999999。
func getProxyBandwidth(proxyURL, suffix string, timeoutSecs int) float64 {
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
	resp, err := client.Get(target)
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
