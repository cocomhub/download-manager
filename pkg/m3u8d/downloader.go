// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package m3u8d

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cavaliergopher/grab/v3"
)

type DownloadConfig struct {
	InputURL    string
	OutputFile  string
	UserAgent   string
	Headers     map[string]string
	Concurrency int
	MaxRetries  int
	WorkDir     string
	KeepFiles   bool
	FFmpegArgs  []string
	Timeout     time.Duration
	Verbose     bool
}

type M3U8Downloader struct {
	Config          *DownloadConfig
	client          *http.Client
	baseURL         *url.URL
	downloaded      map[string]bool
	mu              sync.RWMutex
	totalFiles      int
	downloadedCount int
}

func NewM3U8Downloader(config *DownloadConfig) (*M3U8Downloader, error) {
	// 楠岃瘉杈撳叆URL
	parsedURL, err := url.Parse(config.InputURL)
	if err != nil {
		return nil, fmt.Errorf("鏃犳晥鐨刄RL: %v", err)
	}

	base64URL := base64.URLEncoding.EncodeToString([]byte(parsedURL.String()))

	// 璁剧疆宸ヤ綔鐩綍
	if config.WorkDir == "" {
		config.WorkDir = fmt.Sprintf("download_%s", base64URL[:10])
	}

	// 纭繚宸ヤ綔鐩綍瀛樺湪
	if err := os.MkdirAll(config.WorkDir, 0755); err != nil {
		return nil, fmt.Errorf("鏃犳硶鍒涘缓宸ヤ綔鐩綍: %v", err)
	}

	// 鍒涘缓HTTP瀹㈡埛绔?	client := &http.Client{
		Timeout: config.Timeout,
	}

	return &M3U8Downloader{
		Config:     config,
		client:     client,
		baseURL:    parsedURL,
		downloaded: make(map[string]bool),
	}, nil
}

// 涓嬭浇鏂囦欢锛屾敮鎸侀噸璇?func (d *M3U8Downloader) downloadFileWithRetry(ctx context.Context, fileURL, localPath string) error {
	var lastErr error

	for i := 0; i <= d.Config.MaxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if i > 0 {
			if d.Config.Verbose {
				fmt.Printf("閲嶈瘯 %d/%d: %s\n", i, d.Config.MaxRetries, filepath.Base(localPath))
			}
			time.Sleep(time.Duration(i*i) * time.Second) // 鎸囨暟閫€閬?		}

		if err := d.downloadFile(ctx, fileURL, localPath); err != nil {
			lastErr = err
			continue
		}

		return nil
	}

	return fmt.Errorf("涓嬭浇澶辫触: %v", lastErr)
}

// 涓嬭浇鍗曚釜鏂囦欢
func (d *M3U8Downloader) downloadFile(ctx context.Context, fileURL, localPath string) error {
	// 妫€鏌ユ枃浠舵槸鍚﹀凡瀛樺湪
	if d.isAlreadyDownloaded(fileURL) {
		if d.Config.Verbose {
			fmt.Printf("鏂囦欢宸插瓨鍦? %s\n", filepath.Base(localPath))
		}
		return nil
	}

	// 鍒涘缓璇锋眰
	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		return err
	}

	// 璁剧疆headers
	req.Header.Set("User-Agent", d.Config.UserAgent)
	for k, v := range d.Config.Headers {
		req.Header.Set(k, v)
	}

	// 鍙戦€佽姹?	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, fileURL)
	}

	// 鍒涘缓鏂囦欢
	file, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// 涓嬭浇鏂囦欢
	if _, err := io.Copy(file, resp.Body); err != nil {
		return err
	}

	// 鏍囪涓哄凡涓嬭浇
	d.markAsDownloaded(fileURL)

	return nil
}

// 浣跨敤grab搴撹繘琛屽苟鍙戜笅杞?func (d *M3U8Downloader) downloadFilesConcurrently(ctx context.Context, files []DownloadTask) error {
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

	// 鐩戞帶涓嬭浇杩涘害
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	responses := make([]*grab.Response, 0, len(reqs))

retry:
	// 鍒涘缓鍝嶅簲閫氶亾
	respch := client.DoBatch(d.Config.Concurrency, reqs...)

	completed := 0
	inProgress := 0
	errReqs := make([]*grab.Request, 0)
	for completed < len(reqs) {
		select {
		case <-ticker.C:
			// 鏄剧ず杩涘害
			if d.Config.Verbose && inProgress > 0 {
				fmt.Printf("涓嬭浇涓? %d 涓枃浠禱n", inProgress)
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
						fmt.Printf("涓嬭浇澶辫触: %s - %v\n", filepath.Base(resp.Filename), resp.Err())
					} else {
						fmt.Printf("涓嬭浇瀹屾垚: %s (%.2f%%)\n",
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

	// 妫€鏌ラ敊璇?	var errs []string
	for _, resp := range responses {
		if resp.Err() != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", filepath.Base(resp.Filename), resp.Err()))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("閮ㄥ垎鏂囦欢涓嬭浇澶辫触: %s", strings.Join(errs, ", "))
	}

	return nil
}

// 瑙ｆ瀽m3u8鏂囦欢
func (d *M3U8Downloader) parseM3U8(ctx context.Context, m3u8URL, localPath string, level int) ([]DownloadTask, error) {
	if d.Config.Verbose {
		fmt.Printf("[L%d] 瑙ｆ瀽: %s\n", level, m3u8URL)
	}

	// 涓嬭浇m3u8鏂囦欢
	if err := d.downloadFileWithRetry(ctx, m3u8URL, localPath); err != nil {
		return nil, fmt.Errorf("鏃犳硶涓嬭浇m3u8: %v", err)
	}

	// 璇诲彇m3u8鍐呭
	content, err := os.ReadFile(localPath)
	if err != nil {
		return nil, err
	}

	if err := os.Rename(localPath, localPath+".bak"); err != nil {
		if d.Config.Verbose {
			fmt.Printf("Warning: failed to backup m3u8 file: %v\n", err)
		}
	}

	// 璁＄畻鍩虹URL
	base, err := url.Parse(m3u8URL)
	if err != nil {
		return nil, err
	}

	// 瑙ｆ瀽鍐呭
	lines := strings.Split(string(content), "\n")
	var tasks []DownloadTask
	var modifiedLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			modifiedLines = append(modifiedLines, line)
			continue
		}

		// 娉ㄩ噴琛岋紝鐩存帴淇濈暀
		if strings.HasPrefix(line, "#") {
			// 妫€鏌ユ槸鍚︽槸瀵嗛挜琛?			if strings.Contains(line, "#EXT-X-KEY") {
				// 鎻愬彇瀵嗛挜URL
				if keyURL, ok := extractKeyURL(line); ok {
					absKeyURL := resolveURL(base, keyURL)
					keyFilename := filepath.Base(keyURL)

					tasks = append(tasks, DownloadTask{
						URL:       absKeyURL,
						LocalPath: filepath.Join(d.Config.WorkDir, keyFilename),
						Type:      "key",
					})

					// 淇敼瀵嗛挜琛岋紝鎸囧悜鏈湴鏂囦欢
					localKeyPath := fmt.Sprintf("file://%s", filepath.Join(d.Config.WorkDir, keyFilename))
					line = strings.Replace(line, keyURL, localKeyPath, 1)
				}
			}
			modifiedLines = append(modifiedLines, line)
			continue
		}

		// 澶勭悊璧勬簮琛?		absURL := resolveURL(base, line)
		filename := filepath.Base(line)

		// 妫€鏌ユ枃浠剁被鍨?		switch {
		case strings.Contains(line, ".m3u8"):
			// 宓屽m3u8
			subM3U8Path := filepath.Join(d.Config.WorkDir, filename)
			subTasks, err := d.parseM3U8(ctx, absURL, subM3U8Path, level+1)
			if err != nil {
				return nil, err
			}
			tasks = append(tasks, subTasks...)
			// 淇敼涓烘湰鍦拌矾寰?			modifiedLines = append(modifiedLines, filename)

		case strings.Contains(line, ".ts"):
			// ts鏂囦欢
			tasks = append(tasks, DownloadTask{
				URL:       absURL,
				LocalPath: filepath.Join(d.Config.WorkDir, filename),
				Type:      "ts",
			})
			modifiedLines = append(modifiedLines, filename)

		case strings.Contains(line, ".key") || strings.Contains(line, ".bin"):
			// 瀵嗛挜鏂囦欢
			tasks = append(tasks, DownloadTask{
				URL:       absURL,
				LocalPath: filepath.Join(d.Config.WorkDir, filename),
				Type:      "key",
			})
			modifiedLines = append(modifiedLines, filename)

		default:
			// 鍏朵粬琛岋紝鐩存帴淇濈暀
			modifiedLines = append(modifiedLines, line)
		}
	}

	// 鏇存柊m3u8鏂囦欢
	updatedContent := strings.Join(modifiedLines, "\n")
	if err := os.WriteFile(localPath, []byte(updatedContent), 0644); err != nil {
		return nil, err
	}

	return tasks, nil
}

var ErrNotEnoughFiles = errors.New("m3u8鏂囦欢涓寘鍚殑璧勬簮鏁伴噺涓嶈冻")

// 涓嬭浇鎵€鏈夎祫婧?func (d *M3U8Downloader) DownloadAll(ctx context.Context) (string, error) {
	if d.Config.Verbose {
		fmt.Printf("寮€濮嬩笅杞? %s\n", d.Config.InputURL)
		fmt.Printf("宸ヤ綔鐩綍: %s\n", d.Config.WorkDir)
	}

	// 瑙ｆ瀽涓籱3u8鏂囦欢
	mainM3U8Path := filepath.Join(d.Config.WorkDir, "master.m3u8")
	tasks, err := d.parseM3U8(ctx, d.Config.InputURL, mainM3U8Path, 0)
	if err != nil {
		return "", fmt.Errorf("瑙ｆ瀽m3u8澶辫触: %v", err)
	}

	d.totalFiles = len(tasks)
	if d.Config.Verbose {
		fmt.Printf("鍙戠幇 %d 涓祫婧愰渶瑕佷笅杞絓n", d.totalFiles)
	}

	if d.totalFiles < 10 {
		return mainM3U8Path, ErrNotEnoughFiles
	}

	// 骞跺彂涓嬭浇锛堟渶澶氶噸璇曚竴娆★級
	const maxAttempts = 2
	var lastErr error
	for attempt := range maxAttempts {
		if attempt > 0 {
			fmt.Printf("閲嶈瘯 %d/%d 涓嬭浇鏂囦欢...\n", attempt+1, maxAttempts)
		}
		if err := d.downloadFilesConcurrently(ctx, tasks); err != nil {
			lastErr = err
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return "", fmt.Errorf("涓嬭浇澶辫触: %v", lastErr)
	}

	return mainM3U8Path, nil
}

// 浣跨敤ffmpeg杞崲
func (d *M3U8Downloader) ConvertToMP4(ctx context.Context, localM3U8Path string) error {
	if d.Config.Verbose {
		fmt.Printf("寮€濮嬭浆鐮? %s -> %s\n", localM3U8Path, d.Config.OutputFile)
	}

	// 鏋勫缓ffmpeg鍛戒护
	args := []string{"-y", "-allowed_extensions", "ALL"}

	// 娣诲姞鍗忚鐧藉悕鍗?	args = append(args, "-protocol_whitelist", "file,http,https,tcp,tls,crypto")

	// 娣诲姞杈撳叆鏂囦欢
	args = append(args, "-i", localM3U8Path)

	// 娣诲姞棰濆鍙傛暟
	args = append(args, d.Config.FFmpegArgs...)

	// 娣诲姞杈撳嚭鏂囦欢
	args = append(args, d.Config.OutputFile)

	// 鎵цffmpeg
	cmd := exec.CommandContext(ctx, "ffmpeg", args...) //nolint:gosec // ffmpeg lookup via PATH is standard
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if d.Config.Verbose {
		fmt.Printf("鎵ц鍛戒护: ffmpeg %s\n", strings.Join(args, " "))
	}

	return cmd.Run()
}

// 娓呯悊宸ヤ綔鐩綍
func (d *M3U8Downloader) Cleanup() error {
	if d.Config.KeepFiles {
		fmt.Printf("鏂囦欢淇濈暀鍦? %s\n", d.Config.WorkDir)
		return nil
	}
	return os.RemoveAll(d.Config.WorkDir)
}

// 杈呭姪鍑芥暟
func (d *M3U8Downloader) isAlreadyDownloaded(url string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.downloaded[url]
}

func (d *M3U8Downloader) markAsDownloaded(url string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.downloaded[url] = true
}

// 瑙ｆ瀽瀵嗛挜URL
func extractKeyURL(line string) (string, bool) {
	re := regexp.MustCompile(`URI="([^"]+)"`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1], true
	}
	return "", false
}

// 瑙ｆ瀽鐩稿URL
func resolveURL(base *url.URL, ref string) string {
	if ref == "" {
		return ""
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(refURL).String()
}

// 涓嬭浇浠诲姟缁撴瀯
type DownloadTask struct {
	URL       string
	LocalPath string
	Type      string
}
