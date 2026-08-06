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
	MinFiles    int // 最低资源文件数，低于此值视为无效 m3u8（默认 10）

	// AllowFileProtocol 控制 ffmpeg 协议白名单是否包含 "file" 协议。
	// 开启后允许 ffmpeg 读取本地文件系统作为输入源（如 m3u8 引用本地文件时）。
	// 默认 false 以防范任意文件读取攻击。若需要使用本地 m3u8 文件转码，设为 true。
	AllowFileProtocol bool
}
