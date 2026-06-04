// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package m3u8d

import "time"

// DownloadConfig 配置 M3U8DEngine 的下载行为。
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
