// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package tunnel 提供经 sproxy 加密隧道发起 HTTP 请求的能力。
// 实现为 sproxy pkg/tunnel（最新协议：AES-256-GCM 流式帧 + 重放保护）的薄封装，
// 替代早期自研的 base64-JSON 隧道协议（与 sproxy /tunnel 端点不兼容）。
package tunnel

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	spxytunnel "github.com/cocomhub/sproxy/pkg/tunnel"
)

// SclientConfig 是隧道请求配置（兼容旧调用方）。
type SclientConfig struct {
	ServerURL        string `yaml:"server_url"`
	UploadEndpoint   string `yaml:"upload_endpoint"`
	DownloadEndpoint string `yaml:"download_endpoint"`
	DeleteEndpoint   string `yaml:"delete_endpoint"`
	CheckMD5         bool   `yaml:"check_md5"`
	Timeout          int    `yaml:"timeout"`
	TunnelKey        string `yaml:"tunnel_key"`
	TunnelEndpoint   string `yaml:"tunnel_endpoint"`
}

// TunnelRequest 经 sproxy 加密隧道转发一个 HTTP 请求，返回响应体字符串。
// tunnelURL 指向 sproxy 的 /tunnel 端点（tunnel.NewLocalHandler）。
func TunnelRequest(cfg *SclientConfig, method, targetURL string, headers map[string]string, body string, showHeaders, verbose bool) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("tunnel: nil config")
	}
	if cfg.TunnelKey == "" {
		return "", fmt.Errorf("tunnel: tunnel_key 未配置（sproxy 凭据 Ring 的 SK 派生密钥）")
	}
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	tunnelURL := strings.TrimRight(cfg.ServerURL, "/") + cfg.TunnelEndpoint
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, err := spxytunnel.NewClient(cfg.TunnelKey, tunnelURL, timeout, logger)
	if err != nil {
		return "", fmt.Errorf("创建 sproxy tunnel 客户端失败: %w", err)
	}

	// 构造标准 http.Request（sproxy tunnel.Client.Do 接受标准类型）。
	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequestWithContext(context.Background(), method, targetURL, bodyReader)
	if err != nil {
		return "", fmt.Errorf("构造请求失败: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "[Tunnel] %s %s => %s\n", method, tunnelURL, targetURL)
	}

	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("tunnel 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if showHeaders {
		for k := range resp.Header {
			fmt.Printf("%s: %s\n", k, resp.Header.Get(k))
		}
		fmt.Println()
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("tunnel 响应 %d: %s", resp.StatusCode, truncate(string(b), 512))
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读响应体失败: %w", err)
	}
	return string(b), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
