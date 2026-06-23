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
)

// ProxySelector 瀹氫箟浠ｇ悊閫夋嫨绛栫暐銆?// Select 杩斿洖鏈€浣充唬鐞?URL锛堢┖瀛楃涓茶〃绀虹洿杩烇級锛屼互鍙婂彲鑳界殑閿欒銆?type ProxySelector interface {
	Select(targetURL string) (proxyURL string, err error)
}

// DefaultProxySelector 鏄粯璁ょ殑浠ｇ悊閫夋嫨鍣ㄥ疄鐜帮紝浣跨敤鏂囦欢缂撳瓨 + 鐩磋繛鎺㈡祴 + 甯﹀璇勫垎銆?type DefaultProxySelector struct {
	proxies          []string
	forceProxy       bool
	cacheDir         string
	decisionCacheTTL int // seconds
	probeTimeout     int // seconds
	bandwidthSuffix  string
}

// NewProxySelector 鍒涘缓榛樿浠ｇ悊閫夋嫨鍣紝浣跨敤缁欏畾鐨勪唬鐞嗗垪琛ㄣ€?// 鍏朵綑鍙傛暟浣跨敤鍚堢悊榛樿鍊硷細
//   - 鍐崇瓥缂撳瓨 TTL锛? 绉?//   - 鎺㈡祴瓒呮椂锛? 绉?//   - 甯﹀璺緞鍚庣紑锛?/bandwidth"
func NewProxySelector(proxies []string) *DefaultProxySelector {
	return &DefaultProxySelector{
		proxies:          proxies,
		decisionCacheTTL: 1,
		probeTimeout:     3,
		bandwidthSuffix:  config.DefaultBandwidthPath,
	}
}

// WithForceProxy 璁剧疆鏄惁寮哄埗浣跨敤浠ｇ悊锛堣烦杩囩洿杩炴帰娴嬶級銆?func (ps *DefaultProxySelector) WithForceProxy(v bool) *DefaultProxySelector {
	ps.forceProxy = v
	return ps
}

// WithCache 璁剧疆缂撳瓨鐩綍鍜屽喅绛?TTL锛堢锛夈€?// ttlSecs 涓?0 鏃朵娇鐢ㄩ粯璁ゅ€笺€?func (ps *DefaultProxySelector) WithCache(dir string, ttlSecs int) *DefaultProxySelector {
	ps.cacheDir = dir
	if ttlSecs > 0 {
		ps.decisionCacheTTL = ttlSecs
	}
	return ps
}

// WithProbe 璁剧疆鐩磋繛鎺㈡祴瓒呮椂锛堢锛夈€倀imeoutSecs 涓?0 鏃朵娇鐢ㄩ粯璁ゅ€笺€?func (ps *DefaultProxySelector) WithProbe(timeoutSecs int) *DefaultProxySelector {
	if timeoutSecs > 0 {
		ps.probeTimeout = timeoutSecs
	}
	return ps
}

// WithBandwidthSuffix 璁剧疆浠ｇ悊甯﹀鎺㈡祴璺緞鍚庣紑銆傞粯璁や负 "/bandwidth"銆?func (ps *DefaultProxySelector) WithBandwidthSuffix(s string) *DefaultProxySelector {
	if s != "" {
		ps.bandwidthSuffix = s
	}
	return ps
}

// Select 鎵ц浠ｇ悊閫夋嫨娴佺▼锛?//  1. 鏃犱唬鐞嗛厤缃椂鐩存帴杩斿洖鐩磋繛
//  2. 妫€鏌ユ枃浠剁紦瀛橈紙domain 绮掑害鐨勭洿杩?浠ｇ悊鍐崇瓥锛?//  3. 缂撳瓨鍛戒腑涓斾负鐩磋繛 鈫?鐩磋繛
//  4. 缂撳瓨鍛戒腑浣嗕负浠ｇ悊 鈫?璺宠繃鐩磋繛鎺㈡祴锛岀洿鎺ュ甫瀹芥壂鎻?//  5. 鏈懡涓紦瀛?鈫?鐩磋繛鎺㈡祴 + 甯﹀鎵弿
//  6. 鍐欏叆缂撳瓨
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

	// 妫€鏌ョ紦瀛?	if info, err := os.Stat(cachePath); err == nil {
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

	// 鐩磋繛鎺㈡祴
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

// selectBestProxy 鎵ц甯﹀鎵弿锛岄€夊嚭鏈€浣充唬鐞嗗苟鍐欏叆缂撳瓨銆?func (ps *DefaultProxySelector) selectBestProxy(cachePath string) (string, error) {
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
