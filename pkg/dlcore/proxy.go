// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dlcore

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---- Shared proxy probe functions (used by both dlcore.Client and WgetDownloader) ----

// CheckDirect 检测是否可直接访问目标 URL。返回 true 表示可直连。
func CheckDirect(targetURL string, forceProxy bool, timeoutSecs int) bool {
	if forceProxy {
		return false
	}
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

// GetProxyBandwidth 查询代理的带宽值（数值越小越好），失败时返回 999999。
func GetProxyBandwidth(proxyURL, suffix string, timeoutSecs int) float64 {
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
