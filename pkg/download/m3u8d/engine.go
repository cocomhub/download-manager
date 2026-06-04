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

var ErrNotEnoughFiles = errors.New("m3u8文件中包含的资源数量不足")

// M3U8DEngine 是 m3u8 下载引擎，支持 http.Client 注入。
// 单文件下载（m3u8 解析、密钥下载等）使用注入的 client；
// 并发 TS 分片下载仍使用 grab 的独立 client。
type M3U8DEngine struct {
	Config          *DownloadConfig
	client          *http.Client
	baseURL         *url.URL
	downloaded      map[string]bool
	mu              sync.RWMutex
	totalFiles      int
	downloadedCount int
}

// NewM3U8DEngine 创建 M3U8DEngine 实例。
// httpClient 为可选的 HTTP 客户端；传 nil 时使用默认 client（超时由 cfg.Timeout 决定）。
func NewM3U8DEngine(cfg *DownloadConfig, httpClient *http.Client) (*M3U8DEngine, error) {
	parsedURL, err := url.Parse(cfg.InputURL)
	if err != nil {
		return nil, fmt.Errorf("无效的URL: %v", err)
	}

	base64URL := base64.URLEncoding.EncodeToString([]byte(parsedURL.String()))

	if cfg.WorkDir == "" {
		cfg.WorkDir = fmt.Sprintf("download_%s", base64URL[:10])
	}

	if err := os.MkdirAll(cfg.WorkDir, 0755); err != nil {
		return nil, fmt.Errorf("无法创建工作目录: %v", err)
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

// DownloadAll 下载 m3u8 及其所有分片，返回本地 m3u8 文件路径。
func (d *M3U8DEngine) DownloadAll(ctx context.Context) (string, error) {
	if d.Config.Verbose {
		fmt.Printf("开始下载: %s\n", d.Config.InputURL)
		fmt.Printf("工作目录: %s\n", d.Config.WorkDir)
	}

	mainM3U8Path := filepath.Join(d.Config.WorkDir, "master.m3u8")
	tasks, err := d.parseM3U8(ctx, d.Config.InputURL, mainM3U8Path, 0)
	if err != nil {
		return "", fmt.Errorf("解析m3u8失败: %v", err)
	}

	d.totalFiles = len(tasks)
	if d.Config.Verbose {
		fmt.Printf("发现 %d 个资源需要下载\n", d.totalFiles)
	}

	if d.totalFiles < 10 {
		return mainM3U8Path, ErrNotEnoughFiles
	}

	// 并发下载（最多重试一次）
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
		return "", fmt.Errorf("下载失败: %v", lastErr)
	}

	return mainM3U8Path, nil
}

// ConvertToMP4 使用 ffmpeg 将本地 m3u8 转换为 MP4。
func (d *M3U8DEngine) ConvertToMP4(ctx context.Context, localM3U8Path string) error {
	if d.Config.Verbose {
		fmt.Printf("开始转码: %s -> %s\n", localM3U8Path, d.Config.OutputFile)
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

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if d.Config.Verbose {
		fmt.Printf("执行命令: ffmpeg %s\n", strings.Join(args, " "))
	}

	return cmd.Run()
}

// Cleanup 清理工作目录。若 KeepFiles 为 true 则不删除。
func (d *M3U8DEngine) Cleanup() error {
	if d.Config.KeepFiles {
		fmt.Printf("文件保留在: %s\n", d.Config.WorkDir)
		return nil
	}
	return os.RemoveAll(d.Config.WorkDir)
}

// downloadFile 下载单个文件，使用注入的 http.Client。
func (d *M3U8DEngine) downloadFile(ctx context.Context, fileURL, localPath string) error {
	if d.isAlreadyDownloaded(fileURL) {
		if d.Config.Verbose {
			fmt.Printf("文件已存在: %s\n", filepath.Base(localPath))
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

// downloadFileWithRetry 下载单个文件，支持指数退避重试。
func (d *M3U8DEngine) downloadFileWithRetry(ctx context.Context, fileURL, localPath string) error {
	var lastErr error

	for i := 0; i <= d.Config.MaxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if i > 0 {
			if d.Config.Verbose {
				fmt.Printf("重试 %d/%d: %s\n", i, d.Config.MaxRetries, filepath.Base(localPath))
			}
			time.Sleep(time.Duration(i*i) * time.Second)
		}

		if err := d.downloadFile(ctx, fileURL, localPath); err != nil {
			lastErr = err
			continue
		}

		return nil
	}

	return fmt.Errorf("下载失败: %v", lastErr)
}

// parseM3U8 递归解析 m3u8 文件，收集所有下载任务。
func (d *M3U8DEngine) parseM3U8(ctx context.Context, m3u8URL, localPath string, level int) ([]DownloadTask, error) {
	if d.Config.Verbose {
		fmt.Printf("[L%d] 解析: %s\n", level, m3u8URL)
	}

	if err := d.downloadFileWithRetry(ctx, m3u8URL, localPath); err != nil {
		return nil, fmt.Errorf("无法下载m3u8: %v", err)
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

// 辅助函数

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

// extractKeyURL 从 EXT-X-KEY 行中提取密钥 URL。
func extractKeyURL(line string) (string, bool) {
	re := regexp.MustCompile(`URI="([^"]+)"`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1], true
	}
	return "", false
}

// resolveURL 将相对 URL 解析为绝对 URL。
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