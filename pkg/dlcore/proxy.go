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

// CheckDirect 妫€娴嬫槸鍚﹀彲鐩存帴璁块棶鐩爣 URL銆傝繑鍥?true 琛ㄧず鍙洿杩炪€?func CheckDirect(targetURL string, forceProxy bool, timeoutSecs int) bool {
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

// GetProxyBandwidth 鏌ヨ浠ｇ悊鐨勫甫瀹藉€硷紙鏁板€艰秺灏忚秺濂斤級锛屽け璐ユ椂杩斿洖 999999銆?func GetProxyBandwidth(proxyURL, suffix string, timeoutSecs int) float64 {
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
