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
	// 验证输入URL
	parsedURL, err := url.Parse(config.InputURL)
	if err != nil {
		return nil, fmt.Errorf("无效的URL: %v", err)
	}

	base64URL := base64.URLEncoding.EncodeToString([]byte(parsedURL.String()))

	// 设置工作目录
	if config.WorkDir == "" {
		config.WorkDir = fmt.Sprintf("download_%s", base64URL[:10])
	}

	// 确保工作目录存在
	if err := os.MkdirAll(config.WorkDir, 0755); err != nil {
		return nil, fmt.Errorf("无法创建工作目录: %v", err)
	}

	// 创建HTTP客户端
	client := &http.Client{
		Timeout: config.Timeout,
	}

	return &M3U8Downloader{
		Config:     config,
		client:     client,
		baseURL:    parsedURL,
		downloaded: make(map[string]bool),
	}, nil
}

// 下载文件，支持重试
func (d *M3U8Downloader) downloadFileWithRetry(ctx context.Context, fileURL, localPath string) error {
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
			time.Sleep(time.Duration(i*i) * time.Second) // 指数退避
		}

		if err := d.downloadFile(ctx, fileURL, localPath); err != nil {
			lastErr = err
			continue
		}

		return nil
	}

	return fmt.Errorf("下载失败: %v", lastErr)
}

// 下载单个文件
func (d *M3U8Downloader) downloadFile(ctx context.Context, fileURL, localPath string) error {
	// 检查文件是否已存在
	if d.isAlreadyDownloaded(fileURL) {
		if d.Config.Verbose {
			fmt.Printf("文件已存在: %s\n", filepath.Base(localPath))
		}
		return nil
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		return err
	}

	// 设置headers
	req.Header.Set("User-Agent", d.Config.UserAgent)
	for k, v := range d.Config.Headers {
		req.Header.Set(k, v)
	}

	// 发送请求
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, fileURL)
	}

	// 创建文件
	file, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// 下载文件
	if _, err := io.Copy(file, resp.Body); err != nil {
		return err
	}

	// 标记为已下载
	d.markAsDownloaded(fileURL)

	return nil
}

// 使用grab库进行并发下载
func (d *M3U8Downloader) downloadFilesConcurrently(ctx context.Context, files []DownloadTask) error {
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

	// 监控下载进度
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	responses := make([]*grab.Response, 0, len(reqs))

retry:
	// 创建响应通道
	respch := client.DoBatch(d.Config.Concurrency, reqs...)

	completed := 0
	inProgress := 0
	errReqs := make([]*grab.Request, 0)
	for completed < len(reqs) {
		select {
		case <-ticker.C:
			// 显示进度
			if d.Config.Verbose && inProgress > 0 {
				fmt.Printf("下载中: %d 个文件\n", inProgress)
			}

		case resp := <-respch:
			completed++

			if resp != nil && resp.Err() != nil {
				fmt.Printf("下载失败: %s - %v - %v\n", filepath.Base(resp.Filename), resp.HTTPResponse.Status, resp.Err())
				if resp.HTTPResponse.StatusCode == 472 {
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

	// 检查错误
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

// 解析m3u8文件
func (d *M3U8Downloader) parseM3U8(ctx context.Context, m3u8URL, localPath string, level int) ([]DownloadTask, error) {
	if d.Config.Verbose {
		fmt.Printf("[L%d] 解析: %s\n", level, m3u8URL)
	}

	// 下载m3u8文件
	if err := d.downloadFileWithRetry(ctx, m3u8URL, localPath); err != nil {
		return nil, fmt.Errorf("无法下载m3u8: %v", err)
	}

	// 读取m3u8内容
	content, err := os.ReadFile(localPath)
	if err != nil {
		return nil, err
	}

	os.Rename(localPath, localPath+".bak")

	// 计算基础URL
	base, err := url.Parse(m3u8URL)
	if err != nil {
		return nil, err
	}

	// 解析内容
	lines := strings.Split(string(content), "\n")
	var tasks []DownloadTask
	var modifiedLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			modifiedLines = append(modifiedLines, line)
			continue
		}

		// 注释行，直接保留
		if strings.HasPrefix(line, "#") {
			// 检查是否是密钥行
			if strings.Contains(line, "#EXT-X-KEY") {
				// 提取密钥URL
				if keyURL, ok := extractKeyURL(line); ok {
					absKeyURL := resolveURL(base, keyURL)
					keyFilename := filepath.Base(keyURL)

					tasks = append(tasks, DownloadTask{
						URL:       absKeyURL,
						LocalPath: filepath.Join(d.Config.WorkDir, keyFilename),
						Type:      "key",
					})

					// 修改密钥行，指向本地文件
					localKeyPath := fmt.Sprintf("file://%s", filepath.Join(d.Config.WorkDir, keyFilename))
					line = strings.Replace(line, keyURL, localKeyPath, 1)
				}
			}
			modifiedLines = append(modifiedLines, line)
			continue
		}

		// 处理资源行
		absURL := resolveURL(base, line)
		filename := filepath.Base(line)

		// 检查文件类型
		switch {
		case strings.Contains(line, ".m3u8"):
			// 嵌套m3u8
			subM3U8Path := filepath.Join(d.Config.WorkDir, filename)
			subTasks, err := d.parseM3U8(ctx, absURL, subM3U8Path, level+1)
			if err != nil {
				return nil, err
			}
			tasks = append(tasks, subTasks...)
			// 修改为本地路径
			modifiedLines = append(modifiedLines, filename)

		case strings.Contains(line, ".ts"):
			// ts文件
			tasks = append(tasks, DownloadTask{
				URL:       absURL,
				LocalPath: filepath.Join(d.Config.WorkDir, filename),
				Type:      "ts",
			})
			modifiedLines = append(modifiedLines, filename)

		case strings.Contains(line, ".key") || strings.Contains(line, ".bin"):
			// 密钥文件
			tasks = append(tasks, DownloadTask{
				URL:       absURL,
				LocalPath: filepath.Join(d.Config.WorkDir, filename),
				Type:      "key",
			})
			modifiedLines = append(modifiedLines, filename)

		default:
			// 其他行，直接保留
			modifiedLines = append(modifiedLines, line)
		}
	}

	// 更新m3u8文件
	updatedContent := strings.Join(modifiedLines, "\n")
	if err := os.WriteFile(localPath, []byte(updatedContent), 0644); err != nil {
		return nil, err
	}

	return tasks, nil
}

var ErrNotEnoughFiles = errors.New("m3u8文件中包含的资源数量不足")

// 下载所有资源
func (d *M3U8Downloader) DownloadAll(ctx context.Context) (string, error) {
	if d.Config.Verbose {
		fmt.Printf("开始下载: %s\n", d.Config.InputURL)
		fmt.Printf("工作目录: %s\n", d.Config.WorkDir)
	}

	// 解析主m3u8文件
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

	// 并发下载
	if err := d.downloadFilesConcurrently(ctx, tasks); err != nil {
		if err := d.downloadFilesConcurrently(ctx, tasks); err != nil {
			return "", fmt.Errorf("下载失败: %v", err)
		}
	}

	return mainM3U8Path, nil
}

// 使用ffmpeg转换
func (d *M3U8Downloader) ConvertToMP4(ctx context.Context, localM3U8Path string) error {
	if d.Config.Verbose {
		fmt.Printf("开始转码: %s -> %s\n", localM3U8Path, d.Config.OutputFile)
	}

	// 构建ffmpeg命令
	args := []string{"-y", "-allowed_extensions", "ALL"}

	// 添加协议白名单
	args = append(args, "-protocol_whitelist", "file,http,https,tcp,tls,crypto")

	// 添加输入文件
	args = append(args, "-i", localM3U8Path)

	// 添加额外参数
	args = append(args, d.Config.FFmpegArgs...)

	// 添加输出文件
	args = append(args, d.Config.OutputFile)

	// 执行ffmpeg
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if d.Config.Verbose {
		fmt.Printf("执行命令: ffmpeg %s\n", strings.Join(args, " "))
	}

	return cmd.Run()
}

// 清理工作目录
func (d *M3U8Downloader) Cleanup() error {
	if d.Config.KeepFiles {
		fmt.Printf("文件保留在: %s\n", d.Config.WorkDir)
		return nil
	}
	return os.RemoveAll(d.Config.WorkDir)
}

// 辅助函数
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

// 解析密钥URL
func extractKeyURL(line string) (string, bool) {
	re := regexp.MustCompile(`URI="([^"]+)"`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1], true
	}
	return "", false
}

// 解析相对URL
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

// 下载任务结构
type DownloadTask struct {
	URL       string
	LocalPath string
	Type      string
}
