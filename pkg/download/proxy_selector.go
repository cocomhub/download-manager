// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

const (
	// defaultMaxBandwidth 是代理探测失败时的默认带宽值（数值越大表示越差）。
	defaultMaxBandwidth = math.MaxFloat64
)

// StaticProxySelector 是静态代理列表的选择器实现。
// 它使用文件缓存 + 直连探测 + 带宽评分来选择最佳代理。
type StaticProxySelector struct {
	proxies          []string
	forceProxy       atomic.Bool
	cacheDir         atomic.Value // string
	decisionCacheTTL atomic.Int64 // seconds
	probeTimeout     atomic.Int64 // seconds
	bandwidthSuffix  atomic.Value // string
	probeMu          sync.Mutex   // 保护带宽探测，防止惊群效应
}

// NewStaticProxySelector 创建基于静态代理列表的选择器。
// 默认值：
//   - 决策缓存 TTL：1 秒
//   - 探测超时：3 秒
//   - 带宽路径后缀："/bandwidth"
func NewStaticProxySelector(proxies []string) *StaticProxySelector {
	s := &StaticProxySelector{
		proxies: proxies,
	}
	s.decisionCacheTTL.Store(1)
	s.probeTimeout.Store(3)
	s.bandwidthSuffix.Store(config.DefaultBandwidthPath)
	return s
}

// WithForceProxy 设置是否强制使用代理（跳过直连探测）。
func (s *StaticProxySelector) WithForceProxy(v bool) *StaticProxySelector {
	s.forceProxy.Store(v)
	return s
}

// WithCache 设置代理决策缓存目录和 TTL（秒）。
func (s *StaticProxySelector) WithCache(dir string, ttl int) *StaticProxySelector {
	s.cacheDir.Store(dir)
	s.decisionCacheTTL.Store(int64(ttl))
	return s
}

// WithProbe 设置直连探测超时（秒）。
func (s *StaticProxySelector) WithProbe(timeout int) *StaticProxySelector {
	s.probeTimeout.Store(int64(timeout))
	return s
}

// WithBandwidthSuffix 设置代理带宽探测路径后缀。默认为 "/bandwidth"。
func (s *StaticProxySelector) WithBandwidthSuffix(suffix string) *StaticProxySelector {
	if suffix != "" {
		s.bandwidthSuffix.Store(suffix)
	}
	return s
}

// cachePathForDomain 返回指定域名的缓存文件路径。
func (s *StaticProxySelector) cachePathForDomain(domain string) string {
	cacheBase, _ := s.cacheDir.Load().(string)
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
	ttl := int(s.decisionCacheTTL.Load())
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
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		slog.Warn("Failed to create cache directory for proxy decision", "path", cachePath, logutil.LogKeyError, err)
		return
	}
	// 写入前清理同一目录下的过期缓存文件
	s.cleanStaleCacheEntries(filepath.Dir(cachePath))
	if err := os.WriteFile(cachePath, []byte(decision), 0644); err != nil {
		slog.Warn("Failed to write proxy decision cache", "path", cachePath, logutil.LogKeyError, err)
	}
}

// cleanStaleCacheEntries 扫描缓存目录，删除所有超过 TTL 的缓存文件。
func (s *StaticProxySelector) cleanStaleCacheEntries(dir string) {
	ttl := time.Duration(s.decisionCacheTTL.Load()) * time.Second
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	now := time.Now()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > ttl {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// Select 实现 ProxySelector 接口。
// 返回空字符串表示直连（不使用代理）。
func (s *StaticProxySelector) Select(ctx context.Context, targetURL string, hint *DownloadHint) (string, error) {
	if len(s.proxies) == 0 {
		return "", nil
	}

	// 快照所有原子值，后续 Select 热路径使用局部变量（无锁读）
	forceProxy := s.forceProxy.Load()
	probeTimeout := int(s.probeTimeout.Load())

	u, err := url.Parse(targetURL)
	if err != nil {
		return "", err
	}

	cachePath := s.cachePathForDomain(u.Host)

	// 检查缓存
	if decision, ok := s.readCachedDecision(cachePath); ok {
		if decision == "direct" {
			if forceProxy {
				// forceProxy=true 时忽略直连缓存，继续走代理选择
				goto skipCache
			}
			return "", nil
		}
		return s.selectBestProxy(ctx, cachePath, probeTimeout)
	}

skipCache:
	// 直连探测
	if !forceProxy && checkDirect(ctx, targetURL, probeTimeout) {
		s.writeCacheDecision(cachePath, "direct")
		return "", nil
	}

	return s.selectBestProxy(ctx, cachePath, probeTimeout)
}

// selectBestProxy 执行带宽扫描，选出最佳代理并写入缓存。
func (s *StaticProxySelector) selectBestProxy(ctx context.Context, cachePath string, probeTimeout int) (string, error) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()

	// 二次检查缓存（可能其他 goroutine 已经探测过了）
	if decision, ok := s.readCachedDecision(cachePath); ok {
		if decision == "direct" {
			return "", nil
		}
		// 缓存只存 "direct"/"proxy" 标记，不存具体代理 URL，
		// 所以即使缓存命中 "proxy" 也需要带宽扫描来选出最佳代理。
	}

	bandwidthSuffix, _ := s.bandwidthSuffix.Load().(string)

	bestProxy := ""
	minBandwidth := defaultMaxBandwidth
	for _, p := range s.proxies {
		bw := getProxyBandwidth(ctx, p, bandwidthSuffix, probeTimeout)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, targetURL, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}
	// HEAD 失败，回退到 GET 小量探测
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return false
	}
	getReq.Header.Set("Range", "bytes=0-0")
	resp, err = client.Do(getReq)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent
}

// getProxyBandwidth 查询代理的带宽值（数值越小越好），失败时返回 defaultMaxBandwidth。
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
		return defaultMaxBandwidth
	}
	resp, err := client.Do(hreq)
	if err != nil {
		return defaultMaxBandwidth
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return defaultMaxBandwidth
	}
	val, err := strconv.ParseFloat(strings.TrimSpace(string(body)), 64)
	if err != nil {
		return defaultMaxBandwidth
	}
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return defaultMaxBandwidth
	}
	return val
}
