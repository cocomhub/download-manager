// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dlcore

import "net/http"

type Option func(*Client)

// WithHTTPClient 自定义 HTTP 客户端（含超时、传输配置）
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) { cl.client = c }
}

// WithLoggerDir 设置日志目录（为空则不落盘）
func WithLoggerDir(dir string) Option {
	return func(cl *Client) { cl.logDir = dir }
}

func WithRootDir(root string) Option {
	return func(cl *Client) { cl.rootDir = root }
}

func WithCacheDir(dir string) Option {
	return func(cl *Client) { cl.cacheDir = dir }
}

func WithProxies(proxies []string) Option {
	return func(cl *Client) { cl.proxies = proxies }
}

func WithForceProxy(force bool) Option {
	return func(cl *Client) { cl.forceProxy = force }
}

func WithMaxRetries(n int) Option {
	return func(cl *Client) { cl.maxRetries = n }
}

func WithFFmpegPath(path string) Option {
	return func(cl *Client) { cl.ffmpegPath = path }
}

func WithHLSAutoMarkAsFail(v bool) Option {
	return func(cl *Client) { cl.hlsAutoMarkAsFail = v }
}

func WithDefaultUserAgent(ua string) Option {
	return func(cl *Client) { cl.defaultUserAgent = ua }
}

func WithDisableInjectBrowserLikeHeaders(v bool) Option {
	return func(cl *Client) { cl.disableInjectBrowserLikeHeaders = v }
}

func WithProxyTuning(ttlSecs int, directProbeTimeoutSecs int, bandwidthPathSuffix string) Option {
	return func(cl *Client) {
		cl.proxyDecisionTTLSecs = ttlSecs
		cl.directProbeTimeoutSecs = directProbeTimeoutSecs
		cl.bandwidthPathSuffix = bandwidthPathSuffix
	}
}

func WithProgressTuning(minPercentStep float64, maxIntervalSeconds int) Option {
	return func(cl *Client) {
		cl.progressMinPercentStep = minPercentStep
		cl.progressMaxIntervalSeconds = maxIntervalSeconds
	}
}

func WithFFmpegExtraArgs(args []string) Option {
	return func(cl *Client) { cl.ffmpegExtraArgs = args }
}

func WithMoveIfExists(enabled bool, dir string) Option {
	return func(cl *Client) {
		cl.moveIfExistsEnabled = enabled
		cl.moveIfExistsDir = dir
	}
}

func WithExternalHLSLog(enabled bool, path string) Option {
	return func(cl *Client) {
		cl.externalHLSLogEnabled = enabled
		cl.externalHLSLogPath = path
	}
}
