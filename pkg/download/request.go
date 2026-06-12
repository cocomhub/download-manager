// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "io"

// DownloadHint 携带下载提示信息，帮助 Selector 和 Extractor 做出决策。
type DownloadHint struct {
	FileSize    int64
	ContentType string
	Extractor   string
	Tags        map[string]string
}

// Request 描述一个下载请求，包含目标 URL、保存路径、头信息、进度回调等。
type Request struct {
	URL           string
	SavePath      string
	Headers       map[string]string
	TrackProgress bool
	OnProgress    func(progress float64, downloaded, total int64)
	// OnMetadata 在每次 Extractor 提取到元数据（ETag、checksum 等）时触发。
	// 触发时 req.Metadata[key] 已被设置，回调用于接收方立即持久化，避免 crash 丢失。
	OnMetadata func(key, value string)
	Metadata   map[string]string
	Hint       *DownloadHint
	Result     *DownloadResult // Extractor 填充此字段，调用方读取后显式应用到目标对象
}

// DownloadResult 包含下载完成后的元数据信息。
// Extractor 在下载成功后填充此结构体，不再直接写入 Request.Metadata。
type DownloadResult struct {
	StatusCode    int
	ContentLength int64
	TotalSize     int64
	MD5Base64     string
	MD5Hex        string
	ModTime       string // RFC3339Nano 格式
}

// RangeRequest 描述一个 HTTP Range 请求的偏移量。
type RangeRequest struct {
	Offset int64
}

// TransportRequest 是 Transport 层使用的完整请求描述。
type TransportRequest struct {
	URL      string
	Method   string
	Headers  map[string]string
	Body     []byte
	Range    *RangeRequest
	ProxyURL string
}

// TransportResponse 是 Transport 层返回的响应，包含 Body 和元数据。
type TransportResponse struct {
	Body          io.ReadCloser
	StatusCode    int
	ContentLength int64
	Headers       map[string]string
	ProxyURL      string
}
