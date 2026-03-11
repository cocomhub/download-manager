package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"download-manager/config"
	"download-manager/core"
	"download-manager/model"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

var (
	DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
)

type NativeHTTPDownloader struct {
	logDir     string
	proxies    []string
	cacheFile  string
	forceProxy bool
	maxRetries int
	client     *http.Client
	dLimiter   *DomainLimiter
	active     sync.Map
	ffmpegPath string
}

var _ core.Downloader = &NativeHTTPDownloader{}

func NewNativeHTTPDownloader(cfg config.Downloader) *NativeHTTPDownloader {
	logDir := cfg.LogDir
	if err := os.MkdirAll(logDir, 0755); err != nil {
		slog.Error("Failed to create log directory", "dir", logDir, "error", err)
		logDir = ""
	}

	home, _ := os.UserHomeDir()

	// 创建配置化的 HTTP 客户端 [1,2](@ref)
	client := &http.Client{
		Timeout: 600 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	return &NativeHTTPDownloader{
		logDir:     logDir,
		proxies:    cfg.Proxies,
		cacheFile:  filepath.Join(home, ".config/download-manager/proxy_cache"),
		forceProxy: cfg.ForceProxy,
		maxRetries: cfg.MaxRetries,
		client:     client,
		dLimiter:   NewDomainLimiter(),
		ffmpegPath: cfg.FfmpegPath,
	}
}

func (d *NativeHTTPDownloader) ApplyDomainLimits(limits map[string]int) {
	for host, max := range limits {
		d.dLimiter.Set(host, max)
	}
}

func (d *NativeHTTPDownloader) Name() string {
	return "native_http"
}

func (d *NativeHTTPDownloader) Download(obj *model.DownloadObject, headers map[string]string) error {
	// 复合下载逻辑保持不变
	if filesVal, ok := obj.Extra["files"]; ok && filesVal != nil {
		var fileList []map[string]string

		if files, ok := filesVal.(primitive.A); ok {
			filesVal = []interface{}(files)
		}

		if files, ok := filesVal.([]map[string]string); ok {
			fileList = files
		} else if files, ok := filesVal.([]interface{}); ok {
			for _, f := range files {
				if fm, ok := f.(map[string]interface{}); ok {
					m := make(map[string]string)
					for k, v := range fm {
						if s, ok := v.(string); ok {
							m[k] = s
						}
					}
					fileList = append(fileList, m)
				}
			}
		} else {
			slog.Error("Composite download with unknown files metadata type",
				"type", fmt.Sprintf("%T", filesVal), "task_id", obj.TaskID)
			return fmt.Errorf("composite download error: unknown 'files' metadata type")
		}

		if len(fileList) == 0 {
			return fmt.Errorf("composite download error: 'files' metadata found but empty")
		}

		slog.Info("Starting composite download", "count", len(fileList), "task_id", obj.TaskID)
		for _, fileMap := range fileList {
			url := fileMap["url"]
			path := fileMap["path"]
			fType := fileMap["type"]

			if url == "" || path == "" {
				continue
			}

			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory for composite file: %w", err)
			}

			subObj := &model.DownloadObject{
				URL:      url,
				SavePath: path,
				Metadata: fileMap,
				Extra:    obj.Extra,
			}

			trackProgress := (fType == "video" || len(fileList) == 1)

			if err := d.downloadFile(subObj, trackProgress, obj, headers); err != nil {
				return err
			}
		}
		obj.Progress = 100
		return nil
	}

	// 单文件下载
	return d.downloadFile(obj, true, obj, headers)
}

var (
	ErrNoTry = errors.New("no try left")
)

// downloadFile 使用原生 HTTP 客户端下载文件 [6,7](@ref)
func (d *NativeHTTPDownloader) downloadFile(subObj *model.DownloadObject, trackProgress bool, progressObj *model.DownloadObject, headers map[string]string) (err error) {
	// HLS 场景使用 ffmpeg
	if isHlsURL(subObj.URL) {
		return d.downloadHLSWithFFmpeg(subObj, trackProgress, progressObj, headers)
	}
	// 确保目录存在
	dir := filepath.Dir(subObj.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if subObj.Metadata == nil {
		subObj.Metadata = make(map[string]string)
	}
	if subObj.Extra == nil {
		subObj.Extra = make(map[string]interface{})
	}

	if subObj.Metadata["status"] == model.StatusCompleted {
		slog.Info("File already completed, skipping", "file", subObj.SavePath)
		return nil
	}

	// 准备日志文件
	var logFile string
	var f io.Writer = io.Discard
	if d.logDir != "" {
		logFile = filepath.Join(d.logDir, filepath.Base(subObj.SavePath)+"."+
			time.Now().Format("20060102150405")+".native.log")
		f, err = os.Create(logFile)
		if err != nil {
			slog.Warn("Failed to create log file", "file", logFile, "error", err)
		} else {
			defer f.(*os.File).Close()
		}
	}

	// 确定代理设置
	proxyURL := ""
	if len(d.proxies) > 0 {
		proxyURL, err = d.determineProxy(subObj.URL)
		if err != nil {
			slog.Warn("Proxy selection failed, falling back to direct",
				"url", subObj.URL, "error", err)
		}
	}

	url := subObj.URL
	// 配置 HTTP 客户端 [1,2](@ref)
	client := d.client
	if proxyURL != "" {
		url = strings.TrimPrefix(url, "http://")
		url = strings.TrimPrefix(url, "https://")
		url = proxyURL + "/" + url
		slog.Info("Using proxy", "url", url, "proxy", proxyURL)

		// proxy, err := url.Parse(proxyURL)
		// if err != nil {
		// 	slog.Warn("Invalid proxy URL, using direct connection", "proxy", proxyURL, "error", err)
		// } else {
		// 	// 为本次请求创建带代理的客户端
		// 	client = &http.Client{
		// 		Timeout: 30 * time.Second,
		// 		Transport: &http.Transport{
		// 			Proxy: http.ProxyURL(proxy),
		// 		},
		// 	}
		// 	slog.Info("Using proxy", "url", subObj.URL, "proxy", proxyURL)
		// }
	}

	// 检查文件是否存在以支持断点续传 [10](@ref)
	var startOffset int64 = 0
	fileInfo, err := os.Stat(subObj.SavePath)
	if err == nil && fileInfo.Size() > 0 {
		startOffset = fileInfo.Size()
		slog.Info("Resuming download", "file", subObj.SavePath, "offset", startOffset)
	}

	defer func() {
		if err != nil {
			fmt.Fprintf(f, "%s Download failed: %v\n", time.Now().Format(time.RFC3339Nano), err)
		}
	}()

	cnt := 0

startDownload:

	if d.maxRetries != 0 && cnt >= d.maxRetries {
		fmt.Fprintf(f, "Max retries reached: %d\n", d.maxRetries)
		return fmt.Errorf("max retries reached: %d", d.maxRetries)
	}
	cnt++

	fmt.Fprintf(f, "Requesting URL: %s (Attempt %d)\n\n", url, cnt)
	d.dLimiter.Acquire(subObj.URL)
	defer d.dLimiter.Release(subObj.URL)
	slog.Info("Starting download", "downloader", "native_http",
		"url", url, "path", subObj.SavePath, "log", logFile, "attempt", cnt)

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.active.Store(subObj.URL, cancel)

	// 创建 HTTP 请求 [7,8](@ref)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头模拟浏览器行为 [7](@ref)
	req.Header.Set("accept", "*/*")
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("pragma", "no-cache")
	req.Header.Set("priority", "i")
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="143", "Chromium";v="143", "Not A(Brand";v="24"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("sec-fetch-dest", "video")
	req.Header.Set("sec-fetch-mode", "no-cors")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("user-agent", DefaultUserAgent)

	// 添加自定义请求头
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	var resp *http.Response

	// 设置 Range 头支持断点续传 [10](@ref)
	if startOffset > 0 {
		nreq := req.Clone(ctx)

		printRequestHeaders(f, nreq)

		resp, err = client.Do(nreq)
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusForbidden {
				return fmt.Errorf("%w: HTTP request failed: %w", ErrNoTry, err)
			}
			return fmt.Errorf("HTTP request failed: %w", err)
		}
		defer resp.Body.Close()

		printResponseHeaders(f, resp)

		contentLength, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		if err == nil && (contentLength == startOffset || contentLength == 0 || contentLength == -1 || resp.ContentLength == startOffset) {
			wantMd5 := resp.Header.Get("X-Amz-Meta-Md5chksum")
			if wantMd5 == "" {
				fmt.Fprintf(f, "The file is already fully retrieved; nothing to do.")
				return nil
			}
			base64MD5, hexMD5, err := ComputeFileMD5(subObj.SavePath)
			if err != nil {
				return fmt.Errorf("failed to compute file MD5: %w", err)
			}
			if base64MD5 == wantMd5 {
				fmt.Fprintf(f, "MD5 check passed: %s\n", base64MD5)
				subObj.Metadata["md5_base64"] = base64MD5
				subObj.Metadata["md5_hex"] = hexMD5

				modTimeStr := resp.Header.Get("Last-Modified")
				if modTimeStr != "" {
					modTime, err := time.Parse(time.RFC1123, modTimeStr)
					if err == nil {
						os.Chtimes(subObj.SavePath, modTime, modTime)
					}
					subObj.Metadata["mod_time"] = modTime.Format(time.RFC3339Nano)
				}
				subObj.Metadata["total_size"] = strconv.FormatInt(contentLength, 10)
				subObj.Metadata["status"] = model.StatusCompleted
				return nil
			}

			fmt.Fprintf(f, "MD5 check failed: want %s, got %s\n"+
				"\tTruncating existing file.\n",
				wantMd5, base64MD5)
			startOffset = 0
		} else if contentLength > 0 && contentLength < startOffset {
			fmt.Fprintf(f, "Server responded with 416 Range Not Satisfiable, but file size does not match existing content.\n"+
				"\tTruncating existing file.\n")
			startOffset = 0
		} else {
			resp.Body.Close()
			resp = nil
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
		}
	}

	if resp == nil {
		printRequestHeaders(f, req)

		// 执行请求
		resp, err = client.Do(req)
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusForbidden {
				return fmt.Errorf("%w: HTTP request failed: %w", ErrNoTry, err)
			}
			return fmt.Errorf("HTTP request failed: %w", err)
		}
		defer resp.Body.Close()

		printResponseHeaders(f, resp)
	}

	if strings.Contains(url, "tk") &&
		(resp.ContentLength == 146 || resp.ContentLength == -1) {
		return fmt.Errorf("%w: invalid content length: %d url:%s", ErrNoTry, resp.ContentLength, url)
	}

	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		fmt.Fprintf(f, "Server responded with 416 Range Not Satisfiable, but file size does not match existing content.\n"+
			"\tTruncating existing file.\n")
		startOffset = 0
		resp.Body.Close()
		goto startDownload
	}

	// 检查响应状态
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return fmt.Errorf("HTTP error: %s", resp.Status)
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "text") &&
		(strings.Contains(url, "mp4") || strings.Contains(url, "jpg")) {
		return fmt.Errorf("%w: invalid content type: %s", ErrNoTry, resp.Header.Get("Content-Type"))
	}

	// 处理断点续传的响应 [10](@ref)
	if startOffset > 0 && resp.StatusCode == 200 && resp.Header.Get("Content-Range") == "" {
		fmt.Fprintf(f, "Server doesn't support resume, restarting download\n")
		slog.Info("Server doesn't support resume, restarting download")
		startOffset = 0 // 服务器不支持断点续传，重新开始下载
		resp.Body.Close()
		goto startDownload
	}

	// 获取文件总大小用于进度计算
	var totalSize int64
	if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
		// 从 Content-Range 头获取总大小，格式: bytes 0-1000/2000
		parts := strings.Split(contentRange, "/")
		if len(parts) == 2 {
			totalSize, _ = strconv.ParseInt(parts[1], 10, 64)
		}
	} else {
		totalSize = resp.ContentLength
		if startOffset > 0 {
			totalSize += startOffset
		}
	}

	// 打开文件用于写入
	fileFlags := os.O_CREATE | os.O_WRONLY
	if startOffset > 0 {
		fileFlags |= os.O_APPEND
	} else {
		fileFlags |= os.O_TRUNC
	}

	file, err := os.OpenFile(subObj.SavePath, fileFlags, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// 如果续传，定位到文件末尾
	if startOffset > 0 {
		_, err = file.Seek(0, io.SeekEnd)
		if err != nil {
			return fmt.Errorf("failed to seek file: %w", err)
		}
	}

	// 创建进度跟踪器
	var reader io.Reader = resp.Body
	if trackProgress && progressObj != nil && totalSize > 0 {
		lastProgress := 0.0
		lastDownloaded := startOffset
		lastUpdate := time.Now()
		reader = &progressReader{
			reader:     resp.Body,
			total:      totalSize,
			downloaded: startOffset,
			onProgress: func(progress float64, downloaded, totalSize int64) {
				progressObj.Progress = int(progress)
				if f != nil && (progress-lastProgress > 0.5 || time.Since(lastUpdate) >= 10*time.Second) {
					bps := float64(downloaded-lastDownloaded) / (time.Since(lastUpdate).Seconds())

					index := 0
					suffixs := []string{"B/s", "KB/s", "MB/s"}
					x := float64(1)
					for bps > 1024 && index < len(suffixs)-1 {
						bps /= 1024
						x *= 1024
						index++
					}

					fmt.Fprintf(f, "%s Progress: %.3f%%  %.2f %s expected time: %.2f s\n", time.Now().Format(time.RFC3339Nano), progress, bps, suffixs[index], (float64(totalSize-downloaded) / bps / x))
					lastProgress = progress
					lastDownloaded = downloaded
					lastUpdate = time.Now()
				}
			},
		}
	}

	// 下载文件内容 [6](@ref)
	written, err := io.Copy(file, reader)
	if err != nil {
		d.active.Delete(subObj.URL)
		return fmt.Errorf("failed to write file: %w", err)
	}

	if wantMd5 := resp.Header.Get("X-Amz-Meta-Md5chksum"); wantMd5 != "" {
		base64MD5, hexMD5, err := ComputeFileMD5(subObj.SavePath)
		if err != nil {
			return fmt.Errorf("failed to compute file MD5: %w", err)
		}
		if base64MD5 != wantMd5 {
			fmt.Fprintf(f, "MD5 check failed: want %s, got %s\n"+
				"\tTruncating existing file.\n",
				wantMd5, base64MD5)
			startOffset = 0
			resp.Body.Close()
			goto startDownload
		}
		fmt.Fprintf(f, "MD5 check passed: %s\n", base64MD5)
		subObj.Metadata["md5_base64"] = base64MD5
		subObj.Metadata["md5_hex"] = hexMD5
	}

	if trackProgress && progressObj != nil {
		progressObj.Progress = 100
	}

	modTimeStr := resp.Header.Get("Last-Modified")
	if modTimeStr != "" {
		modTime, err := time.Parse(time.RFC1123, modTimeStr)
		if err == nil {
			os.Chtimes(subObj.SavePath, modTime, modTime)
		}
		subObj.Metadata["mod_time"] = modTime.Format(time.RFC3339Nano)
	}

	if totalSize <= 0 {
		if info, statErr := os.Stat(subObj.SavePath); statErr == nil && info.Size() > 0 {
			totalSize = info.Size()
		} else {
			totalSize = written
		}
	}
	subObj.Metadata["total_size"] = strconv.FormatInt(totalSize, 10)
	subObj.Metadata["status"] = model.StatusCompleted

	// 记录下载完成信息
	fmt.Fprintf(f, "Download completed, total size: %d bytes\n", totalSize)
	slog.Info("Download completed", "file", subObj.SavePath, "size", totalSize)
	d.active.Delete(subObj.URL)
	return nil
}

func isHlsURL(u string) bool {
	lu := strings.ToLower(u)
	return strings.Contains(lu, ".m3u8")
}

func (d *NativeHTTPDownloader) downloadHLSWithFFmpeg(subObj *model.DownloadObject, trackProgress bool, progressObj *model.DownloadObject, headers map[string]string) error {
	dir := filepath.Dir(subObj.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	var logFile string
	var f *os.File
	if d.logDir != "" {
		logFile = filepath.Join(d.logDir, filepath.Base(subObj.SavePath)+"."+time.Now().Format("20060102150405")+".ffmpeg.log")
		var err error
		f, err = os.Create(logFile)
		if err != nil {
			slog.Warn("Failed to create ffmpeg log file", "file", logFile, "error", err)
		} else {
			defer f.Close()
		}
	}

	ffmpeg := d.ffmpegPath
	if strings.TrimSpace(ffmpeg) == "" {
		ffmpeg = "ffmpeg"
	}
	if path, err := exec.LookPath(ffmpeg); err == nil {
		ffmpeg = path
	} else {
		return fmt.Errorf("ffmpeg not found: please install ffmpeg or set downloader.ffmpeg_path")
	}

	args := []string{"-y"}
	if ua := DefaultUserAgent; ua != "" {
		args = append(args, "-user_agent", ua)
	}
	var headerLines []string
	if v := headers["Referer"]; v != "" {
		headerLines = append(headerLines, fmt.Sprintf("Referer: %s", v))
	}
	if v := headers["Cookie"]; v != "" {
		headerLines = append(headerLines, fmt.Sprintf("Cookie: %s", v))
	}
	if len(headerLines) > 0 {
		args = append(args, "-headers", strings.Join(headerLines, "\r\n"))
	}
	args = append(args, "-i", subObj.URL)
	args = append(args, "-c", "copy", "-bsf:a", "aac_adtstoasc", "-movflags", "+faststart", "-f", "mp4", subObj.SavePath)

	ctx, cancel := context.WithCancel(context.Background())
	d.active.Store(subObj.URL, cancel)
	defer d.active.Delete(subObj.URL)

	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg: failed to attach stderr: %w", err)
	}
	cmd.Stdout = f

	slog.Info("Starting download", "downloader", "ffmpeg", "url", subObj.URL, "path", subObj.SavePath, "ffmpeg_log", logFile)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start failed: %w", err)
	}
	go func() {
		io.Copy(f, stderr)
	}()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg execution failed: %w", err)
	}
	if trackProgress && progressObj != nil {
		progressObj.Progress = 100
	}
	subObj.Metadata["status"] = model.StatusCompleted
	if info, err := os.Stat(subObj.SavePath); err == nil {
		subObj.Metadata["total_size"] = strconv.FormatInt(info.Size(), 10)
	}
	return nil
}

func printRequestHeaders(f io.Writer, req *http.Request) {
	fmt.Fprintf(f, "[%s] Request:\n", req.Method)
	fmt.Fprintf(f, "Proto: %s\n", req.Proto)
	fmt.Fprintf(f, "Method: %s\n", req.Method)
	fmt.Fprintf(f, "URL: %s\n", req.URL.String())
	fmt.Fprintf(f, "Headers:\n")
	for k, v := range req.Header {
		fmt.Fprintf(f, "\t%s: %s\n", k, strings.Join(v, ", "))
	}
	fmt.Fprintf(f, "\n")
}

func printResponseHeaders(f io.Writer, resp *http.Response) {
	fmt.Fprintf(f, "[%s] Response:\n", resp.Request.Method)
	fmt.Fprintf(f, "Proto: %s\n", resp.Proto)
	fmt.Fprintf(f, "Status: %s\n", resp.Status)
	fmt.Fprintf(f, "Content-Length: %d\n", resp.ContentLength)
	fmt.Fprintf(f, "Transfer-Encoding: %s\n", resp.TransferEncoding)
	fmt.Fprintf(f, "Connection: %s\n", resp.Header.Get("Connection"))
	fmt.Fprintf(f, "Headers:\n")
	for k, v := range resp.Header {
		fmt.Fprintf(f, "\t%s: %s\n", k, strings.Join(v, ", "))
	}
	fmt.Fprintf(f, "\n")
}

// progressReader 包装器用于跟踪下载进度
type progressReader struct {
	reader     io.Reader
	total      int64
	downloaded int64
	onProgress func(float64, int64, int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.downloaded += int64(n)
		if pr.total > 0 {
			progress := float64(pr.downloaded) / float64(pr.total) * 100
			pr.onProgress(progress, pr.downloaded, pr.total)
		}
	}
	return n, err
}

// 以下代理相关方法保持不变
func (d *NativeHTTPDownloader) determineProxy(targetURL string) (string, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return "", err
	}
	domain := u.Host

	// 缓存逻辑保持不变
	cachePath := filepath.Join(filepath.Dir(d.cacheFile), domain)
	if info, err := os.Stat(cachePath); err == nil {
		if time.Since(info.ModTime()) < 1*time.Second {
			content, _ := os.ReadFile(cachePath)
			s := strings.TrimSpace(string(content))
			if s == "direct" {
				return "", nil
			}
		}
	}

	if d.checkDirect(targetURL) {
		d.writeCache(domain, "direct")
		return "", nil
	}

	bestProxy := ""
	minBandwidth := 999999.0
	for _, p := range d.proxies {
		bw := d.getProxyBandwidth(p)
		if bw < minBandwidth {
			minBandwidth = bw
			bestProxy = p
		}
	}

	if bestProxy != "" {
		d.writeCache(domain, "proxy")
		return bestProxy, nil
	}

	return "", fmt.Errorf("no suitable proxy found")
}

func (d *NativeHTTPDownloader) checkDirect(url string) bool {
	if d.forceProxy {
		return false
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Head(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return true
}

func (d *NativeHTTPDownloader) getProxyBandwidth(proxyURL string) float64 {
	target := fmt.Sprintf("%s/bandwidth", strings.TrimRight(proxyURL, "/"))
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(target)
	if err != nil {
		return 999999
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 999999
	}
	val, err := strconv.ParseFloat(strings.TrimSpace(string(body)), 64)
	if err != nil {
		return 999999
	}
	return val
}

func (d *NativeHTTPDownloader) writeCache(domain, status string) {
	cachePath := filepath.Join(filepath.Dir(d.cacheFile), domain)
	os.MkdirAll(filepath.Dir(cachePath), 0755)
	os.WriteFile(cachePath, []byte(status), 0644)
}

func (d *NativeHTTPDownloader) Cancel(url string) error {
	if v, ok := d.active.Load(url); ok {
		cancel := v.(context.CancelFunc)
		cancel()
		d.active.Delete(url)
		return nil
	}
	return fmt.Errorf("no active download for url")
}
