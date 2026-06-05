// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package m3u8d

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cavaliergopher/grab/v3"
)

// DownloadTask 描述一个需要下载的资源。
type DownloadTask struct {
	URL       string
	LocalPath string
	Type      string
}

// maxRetryRounds 是 downloadFilesConcurrently 内部 retry 循环的最大轮次，
// 防止永久失败请求导致无界循环。
const maxRetryRounds = 3

// downloadFilesConcurrently 使用 grab 库并发下载 TS 分片等资源。
// grab.NewClient() 会创建独立的 http.Client，不受注入 client 影响。
func (d *M3U8DEngine) downloadFilesConcurrently(ctx context.Context, files []DownloadTask) error {
	client := grab.NewClient()

	reqs := make([]*grab.Request, 0, len(files))
	for _, file := range files {
		req, err := grab.NewRequest(file.LocalPath, file.URL)
		if err != nil {
			return err
		}
		req = req.WithContext(ctx)
		req.HTTPRequest.Header.Set("User-Agent", d.Config.UserAgent)
		for k, v := range d.Config.Headers {
			req.HTTPRequest.Header.Set(k, v)
		}
		reqs = append(reqs, req)
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	responses := make([]*grab.Response, 0, len(reqs))

	retryRound := 0
retry:
	// 读 Concurrency 时加锁，避免与 StatusCode==472 时的写操作产生 data race。
	d.concurrencyMu.Lock()
	concurrency := d.Config.Concurrency
	d.concurrencyMu.Unlock()

	respch := client.DoBatch(concurrency, reqs...)

	completed := 0
	errReqs := make([]*grab.Request, 0)
	for completed < len(reqs) {
		select {
		case <-ticker.C:
			if d.Config.Verbose {
				fmt.Printf("下载中: 已完成 %d/%d\n", completed, len(reqs))
			}

		case resp := <-respch:
			completed++

			if resp != nil && resp.Err() != nil {
				status := "unknown"
				if resp.HTTPResponse != nil {
					status = resp.HTTPResponse.Status
				}
				fmt.Printf("下载失败: %s - %s - %v\n", filepath.Base(resp.Filename), status, resp.Err())
				if resp.HTTPResponse != nil && resp.HTTPResponse.StatusCode == 472 {
					d.concurrencyMu.Lock()
					d.Config.Concurrency = 1
					d.concurrencyMu.Unlock()
				}
				req, err := grab.NewRequest(resp.Filename, resp.Request.HTTPRequest.URL.String())
				if err != nil {
					return err
				}
				req = req.WithContext(ctx)
				errReqs = append(errReqs, req)
				continue
			}

			if resp != nil {
				responses = append(responses, resp)
			}

			if resp != nil && resp.HTTPResponse != nil && resp.HTTPResponse.Request != nil {
				d.markAsDownloaded(resp.HTTPResponse.Request.URL.String())

				if d.Config.Verbose {
					fmt.Printf("下载完成: %s\n", filepath.Base(resp.Filename))
				}
			}
		}
	}

	if len(errReqs) > 0 {
		retryRound++
		if retryRound >= maxRetryRounds {
			var failedURLs []string
			for _, r := range errReqs {
				failedURLs = append(failedURLs, r.HTTPRequest.URL.String())
			}
			return fmt.Errorf("超过最大重试轮次 (%d)，%d 个文件下载失败: %s",
				maxRetryRounds, len(errReqs), strings.Join(failedURLs, ", "))
		}
		reqs = errReqs
		goto retry
	}

	return nil
}
