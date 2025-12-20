package downloader

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"download-manager/config"
	"download-manager/core"
	"download-manager/model"
)

type WgetDownloader struct {
	logDir    string
	proxies   []string
	cacheFile string
}

// Ensure WgetDownloader implements core.Downloader
var _ core.Downloader = &WgetDownloader{}

func NewWgetDownloader(cfg config.DownloaderConfig) *WgetDownloader {
	logDir := cfg.LogDir
	if err := os.MkdirAll(logDir, 0755); err != nil {
		slog.Error("Failed to create log directory", "dir", logDir, "error", err)
		logDir = ""
	}

	home, _ := os.UserHomeDir()

	return &WgetDownloader{
		logDir:    logDir,
		proxies:   cfg.Proxies,
		cacheFile: filepath.Join(home, ".config/download-manager/proxy_cache"),
	}
}

func (d *WgetDownloader) Name() string {
	return "wget"
}

func (d *WgetDownloader) Download(obj *model.DownloadObject) error {
	// Check for composite download (files in Extra)
	if filesVal, ok := obj.Extra["files"]; ok {
		var fileList []map[string]string

		// Handle different types depending on source (memory vs JSON)
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
		}

		if len(fileList) > 0 {
			slog.Info("Starting composite download", "count", len(fileList), "task_id", obj.TaskID)
			for _, fileMap := range fileList {
				url := fileMap["url"]
				path := fileMap["path"]
				fType := fileMap["type"]

				if url == "" || path == "" {
					continue
				}

				// Create directory for this file
				dir := filepath.Dir(path)
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("failed to create directory for composite file: %w", err)
				}

				// Construct a temporary object for this file
				subObj := &model.DownloadObject{
					URL:      url,
					SavePath: path,
				}

				// Track progress only for video (main content), or if it's the only file
				trackProgress := (fType == "video" || len(fileList) == 1)

				if err := d.downloadFile(subObj, trackProgress, obj); err != nil {
					return err
				}
			}
			// Ensure 100% at the end
			obj.Progress = 100
			return nil
		}
	}

	// Fallback to standard single file download
	return d.downloadFile(obj, true, obj)
}

var (
	reProgress = regexp.MustCompile(`\s+(\d+)%`)
)

func (d *WgetDownloader) downloadFile(subObj *model.DownloadObject, trackProgress bool, progressObj *model.DownloadObject) error {
	// Ensure directory exists
	dir := filepath.Dir(subObj.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Prepare log file for wget output
	var f *os.File
	var logFile string
	if d.logDir != "" {
		logFile = filepath.Join(d.logDir, filepath.Base(subObj.SavePath)+"."+time.Now().Format("20060102150405")+".wget.log")
		var err error
		f, err = os.Create(logFile)
		if err != nil {
			slog.Warn("Failed to create wget log file", "file", logFile, "error", err)
		} else {
			defer f.Close()
		}
	}

	// Determine connection mode (direct or proxy)
	proxyURL := ""
	if len(d.proxies) > 0 {
		var err error
		proxyURL, err = d.determineProxy(subObj.URL)
		if err != nil {
			slog.Warn("Proxy selection failed, falling back to direct", "url", subObj.URL, "error", err)
		}
	}

	// Build wget command
	args := []string{"-c", "-T", "20", "-t", "5"}

	// Add User-Agent
	args = append(args, "--header", "User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	// Add proxy arguments if selected
	env := os.Environ()
	if proxyURL != "" {
		slog.Info("Using proxy", "url", subObj.URL, "proxy", proxyURL)
		// Set both environment variables and command line args to be safe,
		// but typically environment variables are enough for wget if not using --no-proxy
		// Using -e http_proxy=... works well with wget
		args = append(args, "-e", "use_proxy=yes")
		args = append(args, "-e", "http_proxy="+proxyURL)
		args = append(args, "-e", "https_proxy="+proxyURL)
	} else {
		slog.Debug("Using direct connection", "url", subObj.URL)
	}

	args = append(args, "-O", subObj.SavePath, subObj.URL)

	cmd := exec.Command("wget", args...)
	cmd.Env = env

	// Wget writes progress to stderr
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Also capture stdout just in case
	cmd.Stdout = f

	slog.Info("Starting download", "downloader", "wget", "url", subObj.URL, "path", subObj.SavePath, "wget_log", logFile)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("wget start failed: %w", err)
	}

	// Parse progress from stderr
	scanner := bufio.NewScanner(stderr)

	for scanner.Scan() {
		line := scanner.Text()

		// Write to log file
		if f != nil {
			f.WriteString(line + "\n")
		}

		if trackProgress && progressObj != nil {
			// Extract progress
			matches := reProgress.FindStringSubmatch(line)
			if len(matches) > 1 {
				if p, err := strconv.Atoi(matches[1]); err == nil {
					progressObj.Progress = p
				}
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("wget execution failed: %w", err)
	}

	if trackProgress && progressObj != nil {
		progressObj.Progress = 100
	}

	return nil
}

// --- Proxy Logic ---

func (d *WgetDownloader) determineProxy(targetURL string) (string, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return "", err
	}
	domain := u.Host

	// Check cache
	cachePath := filepath.Join(filepath.Dir(d.cacheFile), domain)
	if info, err := os.Stat(cachePath); err == nil {
		if time.Since(info.ModTime()) < 1*time.Second { // 1s TTL
			content, _ := os.ReadFile(cachePath)
			s := strings.TrimSpace(string(content))
			if s == "direct" {
				return "", nil
			}
			// If cached as proxy, we re-evaluate to find best bandwidth
		}
	}

	// Check Direct
	if d.checkDirect(targetURL) {
		d.writeCache(domain, "direct")
		return "", nil
	}

	// Select Best Proxy
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

func (d *WgetDownloader) checkDirect(url string) bool {
	client := &http.Client{
		Timeout: 3 * time.Second,
	}
	// Use Head to check connectivity quickly
	resp, err := client.Head(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return true
}

func (d *WgetDownloader) getProxyBandwidth(proxyURL string) float64 {
	// Proxy bandwidth endpoint: proxy_url + "/bandwidth"
	target := fmt.Sprintf("%s/bandwidth", strings.TrimRight(proxyURL, "/"))

	client := &http.Client{
		Timeout: 3 * time.Second,
	}
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

func (d *WgetDownloader) writeCache(domain, status string) {
	cachePath := filepath.Join(filepath.Dir(d.cacheFile), domain)
	os.MkdirAll(filepath.Dir(cachePath), 0755)
	os.WriteFile(cachePath, []byte(status), 0644)
}
