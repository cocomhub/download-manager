// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package m3u8d

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/cavaliergopher/grab/v3"
	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/logutil"
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
		// 轮间指数退避（对齐单文件 downloadFileWithRetry：round² 秒），
		// 避免失败分片立即重试放大源站压力（尤其 5xx/超时类）。
		wait := min((retryRound+1)*(retryRound+1), 30)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(wait) * time.Second):
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
		retry, err := d.verifySegment(resp)
		if err != nil {
			return err
		}
		if retry {
			// 校验不匹配：不直接判失败，重建请求进入下一轮重下确认
			// （再次下载后若内容与本次一致则通过，见 verifySegment）。
			if req, rerr := d.rebuildRequest(resp, ctx); rerr != nil {
				return rerr
			} else {
				*errReqs = append(*errReqs, req)
			}
			return nil
		}
		d.recordSuccess(resp)
		return nil
	}
	return d.recordFailure(resp, ctx, errReqs)
}

// rebuildRequest 由已下载响应重建一个带 UA/自定义头的重试请求（对齐 buildGrabRequests）。
// NoResume=true：校验重下确认需拿到完整文件内容比对，禁止对已存在文件续传。
func (d *M3U8DEngine) rebuildRequest(resp *grab.Response, ctx context.Context) (*grab.Request, error) {
	if resp.Request == nil || resp.Request.HTTPRequest == nil {
		return nil, nil
	}
	url := resp.Request.HTTPRequest.URL.String()
	req, err := grab.NewRequest(resp.Filename, url)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	req.NoResume = true
	if d.Config != nil {
		if d.Config.UserAgent != "" {
			req.HTTPRequest.Header.Set("User-Agent", d.Config.UserAgent)
		}
		for k, v := range d.Config.Headers {
			req.HTTPRequest.Header.Set(k, v)
		}
	}
	return req, nil
}

// verifySegment 下载成功后按配置校验分片内容与云端一致（复用 pkg/download 的 MD5 工具）。
// 返回 (retry, err)：
//   - err==nil 且 retry=false → 校验通过/接受（含两一致确认），接受当前分片。
//   - retry=true → 需要再下一次以“两致对接”确认（见 verifyPicked）。
//   - err != nil → 硬错误（读文件失败等），中止。
//
// 决策：
//   - DisableVerifyETag=true → 直接通过（显式禁用校验）。
//   - ETag 为内容 MD5 且与已下载内容一致 → 直通（首下即可靠）。
//   - 其余情况（ETag 缺失 / 非 MD5 / 与内容不符）→ 一律走两致对接（verifyPicked），
//     出现两份内容相同即接受，保证文件可靠，无需白名单。
func (d *M3U8DEngine) verifySegment(resp *grab.Response) (bool, error) {
	if d.Config == nil || d.Config.DisableVerifyETag {
		return false, nil
	}
	if resp == nil || resp.Request == nil || resp.Request.HTTPRequest == nil || resp.HTTPResponse == nil {
		return false, nil
	}
	rawURL := resp.Request.HTTPRequest.URL.String()
	etag := resp.HTTPResponse.Header.Get("ETag")
	want := download.TryGetMd5(map[string]string{"Etag": etag})

	hexMD5, err := d.computeSegmentMD5(resp.Filename)
	if err != nil {
		return false, fmt.Errorf("m3u8d: read segment for verify: %w", err)
	}
	// ETag 为内容 MD5 且内容一致 → 首下即可靠，直接接受。
	if want != "" && download.MD5HexEqual(hexMD5, want) {
		d.rememberVerifyMD5(rawURL, hexMD5)
		if d.Config.Verbose {
			fmt.Printf("校验通过: %s (MD5 %s)\n", filepath.Base(resp.Filename), hexMD5)
		}
		return false, nil
	}
	// 其余（ETag 缺失 / 非 MD5 / 与内容不符）：走两致对接重下比较保证可靠。
	return d.verifyPicked(rawURL, hexMD5, want, resp.Filename)
}

// verifyPicked 校验不匹配/不可用时的“两致对接”决策：
//   - 本次 MD5 与已记录的一致 → 已存在两份相同内容，接受为终文件。
//   - 本次 MD5 与已记录的不同 → 记录最新，返回 retry=true 再下一次，直至出现两份相同。
func (d *M3U8DEngine) verifyPicked(rawURL, hexMD5, want, filename string) (bool, error) {
	if prev, seen := d.verifyMD5.Load(rawURL); seen && download.MD5HexEqual(hexMD5, prev.(string)) {
		slog.Warn("m3u8d: two downloads identical — accepting",
			logutil.LogKeyURL, rawURL, "md5", hexMD5, "etag", want)
		if d.Config.Verbose {
			fmt.Printf("两致对接通过: %s (MD5 %s)\n", filepath.Base(filename), hexMD5)
		}
		return false, nil
	}
	d.verifyMD5.Store(rawURL, hexMD5)
	return true, nil
}

// rememberVerifyMD5 记录某 URL 最近一次（通过校验的）内容 MD5。
// 用于两致对接比对：仅在校验通过/落定后更新，避免覆盖成“待确认”状态。
func (d *M3U8DEngine) rememberVerifyMD5(url, hexMD5 string) {
	d.verifyMD5.Store(url, hexMD5)
}

// computeSegmentMD5 计算分片 MD5。
func (d *M3U8DEngine) computeSegmentMD5(path string) (string, error) {
	_, hexMD5, err := download.ComputeFileMD5(path)
	return hexMD5, err
}

func (d *M3U8DEngine) recordSuccess(resp *grab.Response) {
	if resp == nil {
		return
	}
	if resp.Request == nil {
		return
	}
	d.markAsDownloaded(resp.Request.URL().String())
	if d.Config.Verbose {
		fmt.Printf("下载完成: %s\n", filepath.Base(resp.Filename))
	}
}

func (d *M3U8DEngine) recordFailure(resp *grab.Response, ctx context.Context, errReqs *[]*grab.Request) error {
	status := "unknown"
	var statusCode int
	if resp.HTTPResponse != nil {
		status = resp.HTTPResponse.Status
		statusCode = resp.HTTPResponse.StatusCode
		if resp.HTTPResponse.StatusCode == 472 {
			d.concurrencyMu.Lock()
			d.Config.Concurrency = 1
			d.concurrencyMu.Unlock()
		}
	}
	fmt.Printf("下载失败: %s - %s - %v\n", filepath.Base(resp.Filename), status, resp.Err())

	// 加 nil guard 防止 panic
	if resp.Request == nil || resp.Request.HTTPRequest == nil {
		slog.Warn("grab: response has no associated request, skipping")
		return nil
	}

	url := resp.Request.HTTPRequest.URL.String()

	// 4xx 分类（P1 修复：不再把全部 4xx 当永久 + 静默成功）：
	//   - 可重试 4xx（408/425/429）：源站限流/请求过频，退避后重试可能成功，进入 errReqs。
	//   - 472：源站对当前并发超限，已降并发为 1，继续重试。
	//   - 终态 4xx（401/403/404/410 及其余 4xx）：资源不存在/无权限等，
	//     返回显式错误而非静默 nil，使 downloadFilesConcurrently 失败而非当成功。
	switch statusCode {
	case 408, 425, 429, 472:
		// 可重试：进入 errReqs 参与下一轮重试。
	case 401, 403, 404, 410:
		slog.Warn("grab: terminal 4xx failure, returning error",
			logutil.LogKeyURL, url, "status", statusCode)
		return fmt.Errorf("grab: permanent 4xx failure: HTTP %d (url=%s)", statusCode, url)
	default:
		if statusCode >= 400 && statusCode < 500 {
			slog.Warn("grab: terminal 4xx failure, returning error",
				logutil.LogKeyURL, url, "status", statusCode)
			return fmt.Errorf("grab: permanent 4xx failure: HTTP %d (url=%s)", statusCode, url)
		}
	}

	// 重建请求，补齐 UA 与自定义头（对齐 buildGrabRequests），避免重试丢头（P2 修复）。
	req, err := grab.NewRequest(resp.Filename, url)
	if err != nil {
		return err
	}
	if d.Config != nil {
		if d.Config.UserAgent != "" {
			req.HTTPRequest.Header.Set("User-Agent", d.Config.UserAgent)
		}
		for k, v := range d.Config.Headers {
			req.HTTPRequest.Header.Set(k, v)
		}
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
