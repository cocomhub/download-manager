// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dlcore

import "net/http"

type Option func(*Client)

// WithHTTPClient 鑷畾涔?HTTP 瀹㈡埛绔紙鍚秴鏃躲€佷紶杈撻厤缃級
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) { cl.client = c }
}

// WithLoggerDir 璁剧疆鏃ュ織鐩綍锛堜负绌哄垯涓嶈惤鐩橈級
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

// WithProxySelector 娉ㄥ叆鑷畾涔?ProxySelector 瀹炵幇銆?// 鑻ヤ笉璋冪敤姝ら€夐」锛孨ewClient 灏嗕粠鏃т唬鐞嗛厤缃瓧娈佃嚜鍔ㄦ瀯閫?DefaultProxySelector銆?func WithProxySelector(ps ProxySelector) Option {
	return func(cl *Client) { cl.proxySelector = ps }
}

// WithFilesystem 鍚屾椂璁剧疆 rootDir銆乴ogDir銆乧acheDir銆?func WithFilesystem(rootDir, logDir, cacheDir string) Option {
	return func(cl *Client) {
		cl.rootDir = rootDir
		cl.logDir = logDir
		cl.cacheDir = cacheDir
	}
}

// WithProxy 鍚屾椂璁剧疆浠ｇ悊鍒楄〃鍜屽己鍒朵唬鐞嗘爣蹇椼€?func WithProxy(proxies []string, force bool) Option {
	return func(cl *Client) {
		cl.proxies = proxies
		cl.forceProxy = force
	}
}
