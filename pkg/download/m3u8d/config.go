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
	FFmpegPath  string // ffmpeg 可执行路径（空=PATH 查找 "ffmpeg"）
	Timeout     time.Duration
	Verbose     bool
	MinFiles    int // 最低资源文件数，低于此值视为无效 m3u8（默认 10）

	// ResolutionCRF 按分辨率高度选转码 CRF 的映射（key=像素高度，如 720/1080/2160）。
	// 仅在 FFmpegArgs 含 "{crf}" 占位符时生效：m3u8 主列表档位分辨率解析后，
	// 取 ≤ 实际高度的最大 key 对应的 CRF 替换占位符。无匹配时用 DefaultCRF。
	ResolutionCRF map[int]int
	// DefaultCRF 未匹配分辨率时的回退 CRF（默认 33）。
	DefaultCRF int

	// DisableVerifyETag 禁用下载后校验。默认 false = 默认开启校验（可靠性优先）：
	// 每次下载后按 ETag（若为内容 MD5）直接比对，其余情况走两致对接重下确认。
	// 必须显式传 true 才禁用（如对不需要可靠性的临时源，或源频繁变化导致重下开销大）。
	DisableVerifyETag bool

	// AllowFileProtocol 控制 ffmpeg 协议白名单是否包含 "file" 协议。
	// 开启后允许 ffmpeg 读取本地文件系统作为输入源（如 m3u8 引用本地文件时）。
	// 默认 false 以防范任意文件读取攻击。若需要使用本地 m3u8 文件转码，设为 true。
	AllowFileProtocol bool
}
