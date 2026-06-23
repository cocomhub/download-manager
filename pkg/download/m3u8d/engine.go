// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package m3u8d

import (
	"context"
	"crypto/sha256"
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
)

var ErrNotEnoughFiles = errors.New("m3u8鏂囦欢涓寘鍚殑璧勬簮鏁伴噺涓嶈冻")

// M3U8DEngine 鏄?m3u8 涓嬭浇寮曟搸锛屾敮鎸?http.Client 娉ㄥ叆銆?// 鍗曟枃浠朵笅杞斤紙m3u8 瑙ｆ瀽銆佸瘑閽ヤ笅杞界瓑锛変娇鐢ㄦ敞鍏ョ殑 client锛?// 骞跺彂 TS 鍒嗙墖涓嬭浇浠嶄娇鐢?grab 鐨勭嫭绔?client銆?type M3U8DEngine struct {
	Config        *DownloadConfig
	client        *http.Client
	baseURL       *url.URL
	downloaded    map[string]bool
	mu            sync.RWMutex
	concurrencyMu sync.Mutex
	totalFiles    int
}

// NewM3U8DEngine 鍒涘缓 M3U8DEngine 瀹炰緥銆?// httpClient 涓哄彲閫夌殑 HTTP 瀹㈡埛绔紱浼?nil 鏃朵娇鐢ㄩ粯璁?client锛堣秴鏃剁敱 cfg.Timeout 鍐冲畾锛夈€?func NewM3U8DEngine(cfg *DownloadConfig, httpClient *http.Client) (*M3U8DEngine, error) {
	parsedURL, err := url.Parse(cfg.InputURL)
	if err != nil {
		return nil, fmt.Errorf("鏃犳晥鐨刄RL: %v", err)
	}

	base64URL := base64.URLEncoding.EncodeToString([]byte(parsedURL.String()))

	if cfg.WorkDir == "" {
		cfg.WorkDir = fmt.Sprintf("download_%s", base64URL[:10])
	}

	if err := os.MkdirAll(cfg.WorkDir, 0755); err != nil {
		return nil, fmt.Errorf("鏃犳硶鍒涘缓宸ヤ綔鐩綍: %v", err)
	}

	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: cfg.Timeout,
		}
	}

	return &M3U8DEngine{
		Config:     cfg,
		client:     httpClient,
		baseURL:    parsedURL,
		downloaded: make(map[string]bool),
	}, nil
}

// DownloadAll 涓嬭浇 m3u8 鍙婂叾鎵€鏈夊垎鐗囷紝杩斿洖鏈湴 m3u8 鏂囦欢璺緞銆?func (d *M3U8DEngine) DownloadAll(ctx context.Context) (string, error) {
	if d.Config.Verbose {
		fmt.Printf("寮€濮嬩笅杞? %s\n", d.Config.InputURL)
		fmt.Printf("宸ヤ綔鐩綍: %s\n", d.Config.WorkDir)
	}

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
	for range maxAttempts {
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

// ConvertToMP4 浣跨敤 ffmpeg 灏嗘湰鍦?m3u8 杞崲涓?MP4銆?func (d *M3U8DEngine) ConvertToMP4(ctx context.Context, localM3U8Path string) error {
	if d.Config.Verbose {
		fmt.Printf("寮€濮嬭浆鐮? %s -> %s\n", localM3U8Path, d.Config.OutputFile)
	}

	// Sanitize headers to prevent argv injection
	for k, v := range d.Config.Headers {
		if strings.ContainsAny(k, "\r\n") || strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("invalid header value contains CR/LF")
		}
	}
	// Sanitize output file to prevent argv injection
	if strings.HasPrefix(d.Config.OutputFile, "-") {
		return fmt.Errorf("invalid output file (starts with '-')")
	}

	args := []string{"-y", "-allowed_extensions", "ALL"}
	// Remove "file" from protocol whitelist to prevent arbitrary file read via ffmpeg
	args = append(args, "-protocol_whitelist", "http,https,tcp,tls,crypto")
	args = append(args, "-i", localM3U8Path)
	args = append(args, d.Config.FFmpegArgs...)
	args = append(args, "--", d.Config.OutputFile)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...) //nolint:gosec // ffmpeg lookup via PATH is standard
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if d.Config.Verbose {
		fmt.Printf("鎵ц鍛戒护: ffmpeg %s\n", strings.Join(args, " "))
	}

	return cmd.Run()
}

// Cleanup 娓呯悊宸ヤ綔鐩綍銆傝嫢 KeepFiles 涓?true 鍒欎笉鍒犻櫎銆?func (d *M3U8DEngine) Cleanup() error {
	if d.Config.KeepFiles {
		fmt.Printf("鏂囦欢淇濈暀鍦? %s\n", d.Config.WorkDir)
		return nil
	}
	return os.RemoveAll(d.Config.WorkDir)
}

// downloadFile 涓嬭浇鍗曚釜鏂囦欢锛屼娇鐢ㄦ敞鍏ョ殑 http.Client銆?func (d *M3U8DEngine) downloadFile(ctx context.Context, fileURL, localPath string) error {
	if d.isAlreadyDownloaded(fileURL) {
		if d.Config.Verbose {
			fmt.Printf("鏂囦欢宸插瓨鍦? %s\n", filepath.Base(localPath))
		}
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", d.Config.UserAgent)
	for k, v := range d.Config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, fileURL)
	}

	file, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return err
	}

	d.markAsDownloaded(fileURL)
	return nil
}

// downloadFileWithRetry 涓嬭浇鍗曚釜鏂囦欢锛屾敮鎸佹寚鏁伴€€閬块噸璇曘€?func (d *M3U8DEngine) downloadFileWithRetry(ctx context.Context, fileURL, localPath string) error {
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
			time.Sleep(time.Duration(i*i) * time.Second)
		}

		if err := d.downloadFile(ctx, fileURL, localPath); err != nil {
			lastErr = err
			continue
		}

		return nil
	}

	return fmt.Errorf("涓嬭浇澶辫触: %v", lastErr)
}

// parseM3U8 閫掑綊瑙ｆ瀽 m3u8 鏂囦欢锛屾敹闆嗘墍鏈変笅杞戒换鍔°€?func (d *M3U8DEngine) parseM3U8(ctx context.Context, m3u8URL, localPath string, level int) ([]DownloadTask, error) {
	if d.Config.Verbose {
		fmt.Printf("[L%d] 瑙ｆ瀽: %s\n", level, m3u8URL)
	}

	if err := d.downloadFileWithRetry(ctx, m3u8URL, localPath); err != nil {
		return nil, fmt.Errorf("鏃犳硶涓嬭浇m3u8: %v", err)
	}

	content, err := os.ReadFile(localPath)
	if err != nil {
		return nil, err
	}

	if err := os.Rename(localPath, localPath+".bak"); err != nil {
		if d.Config.Verbose {
			fmt.Printf("Warning: failed to backup m3u8 file: %v\n", err)
		}
	}

	base, err := url.Parse(m3u8URL)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	var tasks []DownloadTask
	var modifiedLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			modifiedLines = append(modifiedLines, line)
			continue
		}

		// Sanitize: reject lines with directory traversal or null bytes
		if strings.Contains(line, "..") || strings.Contains(line, "\x00") {
			return nil, fmt.Errorf("invalid resource path in m3u8: %s", line)
		}

		if strings.HasPrefix(line, "#") {
			if strings.Contains(line, "#EXT-X-KEY") {
				if keyURL, ok := extractKeyURL(line); ok {
					// Validate key URL scheme
					keyParsed, err := url.Parse(keyURL)
					if err != nil || (keyParsed.Scheme != "http" && keyParsed.Scheme != "https") {
						return nil, fmt.Errorf("invalid key URL scheme in m3u8: %s", keyURL)
					}

					absKeyURL := resolveURL(base, keyURL)
					// Use hash-based filename to prevent path traversal
					keyHash := fmt.Sprintf("key_%x", sha256.Sum256([]byte(keyURL)))[:20]
					keyLocalPath := filepath.Join(d.Config.WorkDir, keyHash)

					tasks = append(tasks, DownloadTask{
						URL:       absKeyURL,
						LocalPath: keyLocalPath,
						Type:      "key",
					})

					localKeyPath := fmt.Sprintf("file://%s", keyLocalPath)
					line = strings.Replace(line, keyURL, localKeyPath, 1)
				}
			}
			modifiedLines = append(modifiedLines, line)
			continue
		}

		absURL := resolveURL(base, line)
		// Use hash-based filename for all resources to prevent path traversal
		fileHash := fmt.Sprintf("%x", sha256.Sum256([]byte(line)))[:20]

		switch {
		case strings.Contains(line, ".m3u8"):
			subM3U8Path := filepath.Join(d.Config.WorkDir, fileHash+".m3u8")
			subTasks, err := d.parseM3U8(ctx, absURL, subM3U8Path, level+1)
			if err != nil {
				return nil, err
			}
			tasks = append(tasks, subTasks...)
			modifiedLines = append(modifiedLines, filepath.Base(subM3U8Path))

		case strings.Contains(line, ".ts"):
			tsLocalPath := filepath.Join(d.Config.WorkDir, fileHash+".ts")
			tasks = append(tasks, DownloadTask{
				URL:       absURL,
				LocalPath: tsLocalPath,
				Type:      "ts",
			})
			modifiedLines = append(modifiedLines, filepath.Base(tsLocalPath))

		case strings.Contains(line, ".key") || strings.Contains(line, ".bin"):
			keyLocalPath := filepath.Join(d.Config.WorkDir, fileHash)
			tasks = append(tasks, DownloadTask{
				URL:       absURL,
				LocalPath: keyLocalPath,
				Type:      "key",
			})
			modifiedLines = append(modifiedLines, filepath.Base(line))

		default:
			modifiedLines = append(modifiedLines, line)
		}
	}

	updatedContent := strings.Join(modifiedLines, "\n")
	if err := os.WriteFile(localPath, []byte(updatedContent), 0644); err != nil {
		return nil, err
	}

	return tasks, nil
}

// 杈呭姪鍑芥暟

func (d *M3U8DEngine) isAlreadyDownloaded(url string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.downloaded[url]
}

func (d *M3U8DEngine) markAsDownloaded(url string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.downloaded[url] = true
}

// extractKeyURL 浠?EXT-X-KEY 琛屼腑鎻愬彇瀵嗛挜 URL銆?func extractKeyURL(line string) (string, bool) {
	re := regexp.MustCompile(`URI="([^"]+)"`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1], true
	}
	return "", false
}

// resolveURL 灏嗙浉瀵?URL 瑙ｆ瀽涓虹粷瀵?URL銆?func resolveURL(base *url.URL, ref string) string {
	if ref == "" {
		return ""
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(refURL).String()
}
