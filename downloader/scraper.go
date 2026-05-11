// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/cmd/scraper_get/tunnel"
	"github.com/cocomhub/download-manager/config"
)

func Scrape(url string, cookie string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, config.GetServerConfig().ScraperPath, url)
	if cookie != "" {
		cmd.Args = append(cmd.Args, "-cookie", cookie)
	}
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("scraper(%s) failed: %v, stderr: %s", config.GetServerConfig().ScraperPath, err, stderr.String())
	}
	return out.String(), nil
}

func ScraperNative(url string, cookie string) (string, error) {
	body, err := doScraperNative(url, cookie)
	if err == nil {
		return body, nil
	}
	if !strings.Contains(url, ":18082") {
		url = strings.TrimPrefix(url, "http://")
		url = strings.TrimPrefix(url, "https://")
		url = "http://129.226.212.209:18082/" + url
	}
	return doScraperNative(url, cookie)
}

func doScraperNative(url string, cookie string) (string, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		slog.Error("ScraperNative new request failed", "url", url, "err", err)
		return "", err
	}
	slog.Debug("ScraperNative request created", "url", url)
	req.Header.Set("accept", "*/*")
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("pragma", "no-cache")
	req.Header.Set("priority", "i")
	req.Header.Set("range", "bytes=0-")
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="143", "Chromium";v="143", "Not A(Brand";v="24"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("sec-fetch-dest", "video")
	req.Header.Set("sec-fetch-mode", "no-cors")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("user-agent", DefaultUserAgent)
	if cookie != "" {
		req.Header.Set("cookie", cookie)
	}

	if len(url) > 0 && !strings.Contains(url, "hanime1.me") {
		header := make(map[string]string)
		for k := range req.Header {
			header[k] = req.Header.Get(k)
		}
		return tunnel.TunnelRequest(&tunnel.SclientConfig{
			ServerURL:        "http://129.226.212.209:18082",
			UploadEndpoint:   "/upload",
			DownloadEndpoint: "/download",
			DeleteEndpoint:   "/delete",
			CheckMD5:         false,
			Timeout:          30,
			TunnelKey:        "7693db0059a3c14fd6bfec175c8e2d1d3d821a414aab77c467df06aefb70e3b7",
			TunnelEndpoint:   "/tunnel",
		}, "GET", url, header, "", false, false)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("scraper(%s) failed: %v", url, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
