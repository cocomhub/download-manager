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

// StaticProxySelector 鏄潤鎬佷唬鐞嗗垪琛ㄧ殑閫夋嫨鍣ㄥ疄鐜般€?// 瀹冧娇鐢ㄦ枃浠剁紦瀛?+ 鐩磋繛鎺㈡祴 + 甯﹀璇勫垎鏉ラ€夋嫨鏈€浣充唬鐞嗐€?type StaticProxySelector struct {
	proxies          []string
	forceProxy       bool
	cacheDir         string
	decisionCacheTTL int // seconds
	probeTimeout     int // seconds
	bandwidthSuffix  string
}

// NewStaticProxySelector 鍒涘缓鍩轰簬闈欐€佷唬鐞嗗垪琛ㄧ殑閫夋嫨鍣ㄣ€?// 榛樿鍊硷細
//   - 鍐崇瓥缂撳瓨 TTL锛? 绉?//   - 鎺㈡祴瓒呮椂锛? 绉?//   - 甯﹀璺緞鍚庣紑锛?/bandwidth"
func NewStaticProxySelector(proxies []string) *StaticProxySelector {
	return &StaticProxySelector{
		proxies:          proxies,
		decisionCacheTTL: 1,
		probeTimeout:     3,
		bandwidthSuffix:  config.DefaultBandwidthPath,
	}
}

// WithForceProxy 璁剧疆鏄惁寮哄埗浣跨敤浠ｇ悊锛堣烦杩囩洿杩炴帰娴嬶級銆?func (s *StaticProxySelector) WithForceProxy(v bool) *StaticProxySelector {
	s.forceProxy = v
	return s
}

// WithCache 璁剧疆浠ｇ悊鍐崇瓥缂撳瓨鐩綍鍜?TTL锛堝ぉ鏁帮級銆?func (s *StaticProxySelector) WithCache(dir string, ttl int) *StaticProxySelector {
	s.cacheDir = dir
	s.decisionCacheTTL = ttl
	return s
}

// WithProbe 璁剧疆鐩磋繛鎺㈡祴瓒呮椂锛堢锛夈€?func (s *StaticProxySelector) WithProbe(timeout int) *StaticProxySelector {
	s.probeTimeout = timeout
	return s
}

// Select 瀹炵幇 ProxySelector 鎺ュ彛銆?// 杩斿洖绌哄瓧绗︿覆琛ㄧず鐩磋繛锛堜笉浣跨敤浠ｇ悊锛夈€?func (s *StaticProxySelector) Select(ctx context.Context, targetURL string, hint *DownloadHint) (string, error) {
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
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			cacheDir = os.TempDir()
		}
		cacheBase = filepath.Join(cacheDir, "dm-proxy-cache")
	}
	cachePath := filepath.Join(cacheBase, domain)

	// 妫€鏌ョ紦瀛?	if info, err := os.Stat(cachePath); err == nil {
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
				return s.selectBestProxy(ctx, cachePath)
			}
		}
	}

	// 鐩磋繛鎺㈡祴
	if !s.forceProxy {
		if checkDirect(ctx, targetURL, s.probeTimeout) {
			_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
			_ = os.WriteFile(cachePath, []byte("direct"), 0644)
			return "", nil
		}
	}

	return s.selectBestProxy(ctx, cachePath)
}

// selectBestProxy 鎵ц甯﹀鎵弿锛岄€夊嚭鏈€浣充唬鐞嗗苟鍐欏叆缂撳瓨銆?func (s *StaticProxySelector) selectBestProxy(ctx context.Context, cachePath string) (string, error) {
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
		_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
		_ = os.WriteFile(cachePath, []byte("proxy"), 0644)
		return bestProxy, nil
	}
	return "", fmt.Errorf("no suitable proxy found")
}

// checkDirect 妫€娴嬫槸鍚﹀彲鐩存帴璁块棶鐩爣 URL銆傝繑鍥?true 琛ㄧず鍙洿杩炪€?func checkDirect(ctx context.Context, targetURL string, timeoutSecs int) bool {
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

// getProxyBandwidth 鏌ヨ浠ｇ悊鐨勫甫瀹藉€硷紙鏁板€艰秺灏忚秺濂斤級锛屽け璐ユ椂杩斿洖 999999銆?func getProxyBandwidth(ctx context.Context, proxyURL, suffix string, timeoutSecs int) float64 {
	if strings.TrimSpace(suffix) == "" {
		suffix = config.DefaultBandwidthPath
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
