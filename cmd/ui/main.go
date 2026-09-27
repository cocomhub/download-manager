// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package main 独立 UI 进程：内嵌 web/static 静态资源 + 反向代理 API/文件到下载进程。
// 更新 UI 只需重建本二进制，下载进程零重启。
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/cocomhub/download-manager/web"
)

var (
	Version = "dev"
	BuildAt = "unknown"
)

// buildHandler 构造 UI 路由：/api/*、/files/* 反代到 backendURL，其余静态资源本进程提供。
func buildHandler(backendURL string) (http.Handler, error) {
	apiURL, err := url.Parse(backendURL)
	if err != nil {
		return nil, fmt.Errorf("invalid --api-url %q: %w", backendURL, err)
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(apiURL)
			pr.SetXForwarded() // 设置 X-Forwarded-For/Host/Proto（后端 SSE 同源检查依赖）
			// Host 头由 SetURL 置空 → 转发用后端 host
			// Authorization / Origin / body 默认透传
		},
		FlushInterval: 100 * time.Millisecond, // SSE 流式透传
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	mux.Handle("/files/", proxy)

	subFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("failed to embed static files: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	return mux, nil
}

func main() {
	var (
		port    = flag.Int("port", 9000, "UI listen port")
		apiURL  = flag.String("api-url", "http://127.0.0.1:8080", "downloader backend API URL")
		showVer = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("Version: %s, Build At: %s\n", Version, BuildAt)
		os.Exit(0)
	}

	handler, err := buildHandler(*apiURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: handler,
	}
	slog.Info("UI server started", "port", *port, "api_url", *apiURL, "version", Version, "build_at", BuildAt)
	slog.Info("Web UI available", "url", fmt.Sprintf("http://localhost:%d", *port))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("UI server failed", "error", err)
		os.Exit(1)
	}
}
