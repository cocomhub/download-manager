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

// CheckBandwidth 探测目标 URL 的带宽（字节/秒）。
// 下载一定字节后计算下载速率。
func CheckBandwidth(ctx context.Context, url string, probeBytes int64, timeout time.Duration) (float64, error) {
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

// CheckHealth 检查代理/隧道节点是否健康。
// 向 url 发 GET 请求，返回 200 OK 时表示健康。
func CheckHealth(ctx context.Context, url string, timeout time.Duration) error {
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
