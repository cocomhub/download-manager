// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/cocomhub/download-manager/cmd/scraper_get/tunnel"
	"github.com/cocomhub/download-manager/downloader"
)

var (
	downloadURL  = flag.String("url", "", "URL to download")
	tunnelURL    = flag.String("tunnel", "", "Tunnel URL锛堥粯璁ょ┖锛岄渶杩愯鏃朵紶鍏ワ級")
	proxyURL     = flag.String("proxy", "", "Proxy URL锛堥粯璁ょ┖锛岄渶杩愯鏃朵紶鍏ワ級")
	tunnelSecret = flag.String("tunnel_key", "", "Tunnel key锛堥粯璁ょ┖锛岄渶杩愯鏃朵紶鍏ワ級")
	cookie       = flag.String("cookie", "", "Cookie string")
)

func main() {
	flag.Parse()

	if *downloadURL == "" && len(flag.Args()) > 0 {
		*downloadURL = flag.Args()[0]
	}

	err := httpGet(*downloadURL)
	if err == nil {
		return
	}

	url := *downloadURL
	if !strings.Contains(url, ":18082") {
		url = strings.TrimPrefix(url, "http://")
		url = strings.TrimPrefix(url, "https://")
		url = *proxyURL + "/" + url
	}

	err = httpGet(url)
	if err != nil {
		log.Fatal(err)
	}
}

func httpGet(url string) error {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}
	if *cookie != "" {
		req.Header.Set("cookie", *cookie)
	}
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
	req.Header.Set("user-agent", downloader.DefaultUserAgent)

	if *tunnelURL != "" && !strings.Contains(url, "hanime1") {
		header := make(map[string]string)
		for k := range req.Header {
			header[k] = req.Header.Get(k)
		}
		body, err := tunnel.TunnelRequest(&tunnel.SclientConfig{
			ServerURL:        *tunnelURL,
			UploadEndpoint:   "/upload",
			DownloadEndpoint: "/download",
			DeleteEndpoint:   "/delete",
			CheckMD5:         false,
			Timeout:          30,
			TunnelKey:        *tunnelSecret,
			TunnelEndpoint:   "/tunnel",
		}, "GET", url, header, "", false, false)
		if err != nil {
			return err
		}
		_, err = io.Copy(os.Stdout, strings.NewReader(body))
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %d", url, resp.StatusCode)
	}

	_, err = io.Copy(os.Stdout, resp.Body)
	return err
}
