package downloader

import (
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
	"time"

	"download-manager/config"
	"download-manager/core"
	"download-manager/model"
)

type ProxyWgetDownloader struct {
	proxies   []string
	logDir    string
	cacheFile string
}

// Ensure ProxyWgetDownloader implements core.Downloader
var _ core.Downloader = &ProxyWgetDownloader{}

func NewProxyWgetDownloader(cfg config.DownloaderConfig) *ProxyWgetDownloader {
	home, _ := os.UserHomeDir()

	// Default proxies if config empty
	proxies := cfg.Proxies
	if len(proxies) == 0 {
		proxies = []string{
			"http://129.226.212.209:18080",
			"http://43.159.49.114:18080",
		}
	}

	logDir := cfg.LogDir
	if err := os.MkdirAll(logDir, 0755); err != nil {
		slog.Error("Failed to create log directory", "dir", logDir, "error", err)
		logDir = ""
	}

	return &ProxyWgetDownloader{
		proxies:   proxies,
		logDir:    logDir,
		cacheFile: filepath.Join(home, ".config/myproxy/cache"),
	}
}

func (d *ProxyWgetDownloader) Name() string {
	return "wget_proxy"
}

func (d *ProxyWgetDownloader) Download(obj *model.DownloadObject) error {
	// Check for composite files in Extra
	if files, ok := obj.Extra["files"]; ok {
		// Handle list of files
		// We expect files to be []map[string]string or similar from JSON unmarshal/manual creation
		// In TktubeTask we created []map[string]string

		// If it's interface{}, we need to reflect or cast carefully.
		// Since we control TktubeTask, we know it's []map[string]string.
		// But if loaded from JSON storage, it might be []interface{} of map[string]interface{}.

		// Let's iterate.
		// Since Go's type system is strict, and Extra is map[string]interface{},
		// We need to handle the type assertion carefully.

		fileList, ok := files.([]map[string]string)
		if !ok {
			// Try []interface{} for generic unmarshal
			if rawList, ok := files.([]interface{}); ok {
				for _, item := range rawList {
					if fileMap, ok := item.(map[string]interface{}); ok {
						urlStr, _ := fileMap["url"].(string)
						pathStr, _ := fileMap["path"].(string)
						if urlStr != "" && pathStr != "" {
							if err := d.downloadSingle(urlStr, pathStr); err != nil {
								return err
							}
						}
					}
				}
				return nil
			}
			// If not matching, fallback to single?
		} else {
			for _, f := range fileList {
				if err := d.downloadSingle(f["url"], f["path"]); err != nil {
					return err
				}
			}
			return nil
		}
	}

	// Fallback to standard single object
	return d.downloadSingle(obj.URL, obj.SavePath)
}

func (d *ProxyWgetDownloader) downloadSingle(targetURL, savePath string) error {
	if targetURL == "" || savePath == "" {
		return fmt.Errorf("invalid url or save path")
	}

	// Ensure directory exists
	dir := filepath.Dir(savePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Prepare log file for wget output
	var f *os.File
	var logFile string
	if d.logDir != "" {
		logFile = filepath.Join(d.logDir, filepath.Base(savePath)+"."+time.Now().Format("20060102150405")+".wget.log")
		var err error
		f, err = os.Create(logFile)
		if err != nil {
			slog.Warn("Failed to create wget log file", "file", logFile, "error", err)
			// Proceed without logging to file if creation fails
		} else {
			defer f.Close()
		}
	}

	// Determine connection mode (direct or proxy)
	proxyURL, err := d.determineProxy(targetURL)
	if err != nil {
		slog.Warn("Proxy selection failed, falling back to direct", "downloader", "proxy_wget", "error", err)
	}

	// Build wget command
	args := []string{"-c", "-T", "20", "-t", "20", "--limit-rate=50m"}
	args = append(args, "--header", "User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	// Add proxy if selected
	env := os.Environ()
	if proxyURL != "" {
		slog.Info("Using proxy", "downloader", "proxy_wget", "proxy", proxyURL)
		args = append(args, "-e", "use_proxy=yes")
		args = append(args, "-e", "http_proxy="+proxyURL)
		args = append(args, "-e", "https_proxy="+proxyURL)
	} else {
		slog.Info("Using direct connection", "downloader", "proxy_wget")
	}

	args = append(args, "-O", savePath, targetURL)

	cmd := exec.Command("wget", args...)
	cmd.Env = env

	// Redirect output to log file if available
	if f != nil {
		cmd.Stdout = f
		cmd.Stderr = f
		slog.Info("Starting download", "downloader", "proxy_wget", "url", targetURL, "path", savePath, "wget_log", logFile)
	} else {
		cmd.Stdout = nil
		cmd.Stderr = nil
		slog.Info("Starting download", "downloader", "proxy_wget", "url", targetURL, "path", savePath)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wget execution failed: %w", err)
	}

	return nil
}

func (d *ProxyWgetDownloader) determineProxy(targetURL string) (string, error) {
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
			// If cached proxy, verify if it's still good? Script re-evaluates.
			// Script says: if cached_result == direct, use direct.
			// If cached_result != direct (implied proxy), "re-select lowest bandwidth proxy".
			// So we only cache "direct".
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

func (d *ProxyWgetDownloader) checkDirect(url string) bool {
	client := &http.Client{
		Timeout: 3 * time.Second,
	}
	resp, err := client.Head(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	// We don't care about status code, just connectivity? Script says "connect-timeout", "--fail".
	// "--fail" implies 2xx-3xx? No, curl --fail fails on server errors.
	// Script comments: "Only check connection established (not verify HTTP status code)" -> wait, script says `curl ... --fail ...`.
	// Actually `curl --fail` fails on HTTP errors.
	// But `check_direct` comment says "use curl only request header... only check connection established".
	// But `curl --fail` contradicts "not verify HTTP status code".
	// Let's assume we need a successful connection.
	return true
}

func (d *ProxyWgetDownloader) getProxyBandwidth(proxyURL string) float64 {
	// Proxy bandwidth endpoint: proxy_url + "/bandwidth"
	// Assumes proxy_url doesn't have trailing slash
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

func (d *ProxyWgetDownloader) writeCache(domain, status string) {
	cachePath := filepath.Join(filepath.Dir(d.cacheFile), domain)
	os.MkdirAll(filepath.Dir(cachePath), 0755)
	os.WriteFile(cachePath, []byte(status), 0644)
}
