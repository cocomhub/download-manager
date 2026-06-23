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

// DownloadTask 鎻忚堪涓€涓渶瑕佷笅杞界殑璧勬簮銆?type DownloadTask struct {
	URL       string
	LocalPath string
	Type      string
}

// maxRetryRounds 鏄?downloadFilesConcurrently 鍐呴儴 retry 寰幆鐨勬渶澶ц疆娆★紝
// 闃叉姘镐箙澶辫触璇锋眰瀵艰嚧鏃犵晫寰幆銆?const maxRetryRounds = 3

// downloadFilesConcurrently 浣跨敤 grab 搴撳苟鍙戜笅杞?TS 鍒嗙墖绛夎祫婧愩€?// grab.NewClient() 浼氬垱寤虹嫭绔嬬殑 http.Client锛屼笉鍙楁敞鍏?client 褰卞搷銆?func (d *M3U8DEngine) downloadFilesConcurrently(ctx context.Context, files []DownloadTask) error {
	client := grab.NewClient()
	if d.client != nil {
		client.HTTPClient = d.client
	}

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

	retryRound := 0
retry:
	// 璇?Concurrency 鏃跺姞閿侊紝閬垮厤涓?StatusCode==472 鏃剁殑鍐欐搷浣滀骇鐢?data race銆?	d.concurrencyMu.Lock()
	concurrency := d.Config.Concurrency
	d.concurrencyMu.Unlock()

	respch := client.DoBatch(concurrency, reqs...)

	completed := 0
	errReqs := make([]*grab.Request, 0)
	for completed < len(reqs) {
		select {
		case <-ticker.C:
			if d.Config.Verbose {
				fmt.Printf("涓嬭浇涓? 宸插畬鎴?%d/%d\n", completed, len(reqs))
			}

		case resp := <-respch:
			completed++

			if resp != nil && resp.Err() != nil {
				status := "unknown"
				if resp.HTTPResponse != nil {
					status = resp.HTTPResponse.Status
				}
				fmt.Printf("涓嬭浇澶辫触: %s - %s - %v\n", filepath.Base(resp.Filename), status, resp.Err())
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

			if resp != nil && resp.HTTPResponse != nil && resp.HTTPResponse.Request != nil {
				d.markAsDownloaded(resp.HTTPResponse.Request.URL.String())

				if d.Config.Verbose {
					fmt.Printf("涓嬭浇瀹屾垚: %s\n", filepath.Base(resp.Filename))
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
			return fmt.Errorf("瓒呰繃鏈€澶ч噸璇曡疆娆?(%d)锛?d 涓枃浠朵笅杞藉け璐? %s",
				maxRetryRounds, len(errReqs), strings.Join(failedURLs, ", "))
		}
		reqs = errReqs
		goto retry
	}

	return nil
}
