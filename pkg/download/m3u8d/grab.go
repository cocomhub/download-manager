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

retry:
	respch := client.DoBatch(d.Config.Concurrency, reqs...)

	completed := 0
	inProgress := 0
	errReqs := make([]*grab.Request, 0)
	for completed < len(reqs) {
		select {
		case <-ticker.C:
			if d.Config.Verbose && inProgress > 0 {
				fmt.Printf("下载中: %d 个文件\n", inProgress)
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
					d.Config.Concurrency = 1
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
				d.downloadedCount++

				if d.Config.Verbose {
					if resp.Err() != nil {
						fmt.Printf("下载失败: %s - %v\n", filepath.Base(resp.Filename), resp.Err())
					} else {
						fmt.Printf("下载完成: %s (%.2f%%)\n",
							filepath.Base(resp.Filename),
							100*float64(d.downloadedCount)/float64(d.totalFiles))
					}
				}
			}
		}
	}

	if len(errReqs) > 0 {
		reqs = errReqs
		goto retry
	}

	var errs []string
	for _, resp := range responses {
		if resp.Err() != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", filepath.Base(resp.Filename), resp.Err()))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("部分文件下载失败: %s", strings.Join(errs, ", "))
	}

	return nil
}
