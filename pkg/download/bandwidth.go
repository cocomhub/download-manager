// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CheckBandwidth 鎺㈡祴鐩爣 URL 鐨勫甫瀹斤紙瀛楄妭/绉掞級銆?// 涓嬭浇涓€瀹氬瓧鑺傚悗璁＄畻涓嬭浇閫熺巼銆?func CheckBandwidth(ctx context.Context, url string, probeBytes int64, timeout time.Duration) (float64, error) {
	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	hreq, err := http.NewRequestWithContext(dctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("bandwidth probe: %w", err)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(hreq)
	if err != nil {
		return 0, fmt.Errorf("bandwidth probe: %w", err)
	}
	defer resp.Body.Close()

	start := time.Now()
	var total int64
	buf := make([]byte, 32*1024)
	for total < probeBytes {
		n, readErr := resp.Body.Read(buf)
		total += int64(n)
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return 0, fmt.Errorf("bandwidth probe: read error: %w", readErr)
		}
	}
	elapsed := time.Since(start)
	if elapsed <= 0 {
		elapsed = time.Nanosecond // avoid division by zero on fast local probes
	}
	return float64(total) / elapsed.Seconds(), nil
}

// CheckHealth 妫€鏌ヤ唬鐞?闅ч亾鑺傜偣鏄惁鍋ュ悍銆?// 鍚?url 鍙?GET 璇锋眰锛岃繑鍥?200 OK 鏃惰〃绀哄仴搴枫€?func CheckHealth(ctx context.Context, url string, timeout time.Duration) error {
	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	hreq, err := http.NewRequestWithContext(dctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(hreq)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check: unexpected status %d", resp.StatusCode)
	}
	return nil
}
