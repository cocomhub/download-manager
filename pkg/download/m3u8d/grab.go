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
	client := d.newGrabClient()
	reqs, err := d.buildGrabRequests(ctx, files)
	if err != nil {
		return err
	}

	for retryRound := range maxRetryRounds {
		errReqs, err := d.runDownloadBatch(ctx, client, reqs)
		if err != nil {
			return err
		}
		if len(errReqs) == 0 {
			return nil
		}
		if retryRound >= maxRetryRounds-1 {
			return formatRetryError(errReqs)
		}
		reqs = errReqs
	}
	return nil
}

func (d *M3U8DEngine) newGrabClient() *grab.Client {
	client := grab.NewClient()
	if d.client != nil {
		client.HTTPClient = d.client
	}
	return client
}

func (d *M3U8DEngine) buildGrabRequests(ctx context.Context, files []DownloadTask) ([]*grab.Request, error) {
	reqs := make([]*grab.Request, 0, len(files))
	for _, file := range files {
		req, err := grab.NewRequest(file.LocalPath, file.URL)
		if err != nil {
			return nil, err
		}
		req = req.WithContext(ctx)
		req.HTTPRequest.Header.Set("User-Agent", d.Config.UserAgent)
		for k, v := range d.Config.Headers {
			req.HTTPRequest.Header.Set(k, v)
		}
		reqs = append(reqs, req)
	}
	return reqs, nil
}

func (d *M3U8DEngine) getBatchConcurrency() int {
	d.concurrencyMu.Lock()
	defer d.concurrencyMu.Unlock()
	return d.Config.Concurrency
}

func (d *M3U8DEngine) runDownloadBatch(ctx context.Context, client *grab.Client, reqs []*grab.Request) ([]*grab.Request, error) {
	concurrency := d.getBatchConcurrency()
	respch := client.DoBatch(concurrency, reqs...)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var errReqs []*grab.Request
	completed := 0

	for completed < len(reqs) {
		select {
		case <-ticker.C:
			if d.Config.Verbose {
				fmt.Printf("下载中: 已完成 %d/%d\n", completed, len(reqs))
			}

		case resp := <-respch:
			completed++
			if err := d.handleBatchResponse(resp, ctx, &errReqs); err != nil {
				return nil, err
			}
		}
	}
	return errReqs, nil
}

func (d *M3U8DEngine) handleBatchResponse(resp *grab.Response, ctx context.Context, errReqs *[]*grab.Request) error {
	if resp == nil || resp.Err() == nil {
		d.recordSuccess(resp)
		return nil
	}
	return d.recordFailure(resp, ctx, errReqs)
}

func (d *M3U8DEngine) recordSuccess(resp *grab.Response) {
	if resp == nil {
		return
	}
	if resp.HTTPResponse == nil {
		return
	}
	if resp.HTTPResponse.Request == nil {
		return
	}
	d.markAsDownloaded(resp.HTTPResponse.Request.URL.String())
	if d.Config.Verbose {
		fmt.Printf("下载完成: %s\n", filepath.Base(resp.Filename))
	}
}

func (d *M3U8DEngine) recordFailure(resp *grab.Response, ctx context.Context, errReqs *[]*grab.Request) error {
	status := "unknown"
	if resp.HTTPResponse != nil {
		status = resp.HTTPResponse.Status
		if resp.HTTPResponse.StatusCode == 472 {
			d.concurrencyMu.Lock()
			d.Config.Concurrency = 1
			d.concurrencyMu.Unlock()
		}
	}
	fmt.Printf("下载失败: %s - %s - %v\n", filepath.Base(resp.Filename), status, resp.Err())

	req, err := grab.NewRequest(resp.Filename, resp.Request.HTTPRequest.URL.String())
	if err != nil {
		return err
	}
	*errReqs = append(*errReqs, req.WithContext(ctx))
	return nil
}

func formatRetryError(errReqs []*grab.Request) error {
	var failedURLs []string
	for _, r := range errReqs {
		failedURLs = append(failedURLs, r.HTTPRequest.URL.String())
	}
	return fmt.Errorf("超过最大重试轮次 (%d)，%d 个文件下载失败: %s",
		maxRetryRounds, len(errReqs), strings.Join(failedURLs, ", "))
}
