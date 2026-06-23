// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "io"

// ProgressReader 包装 io.Reader，在读取过程中触发进度回调。
// 适用于下载场景中实时报告已读取字节数占总字节数的比例。
type ProgressReader struct {
	reader     io.Reader
	total      int64
	downloaded int64
	onProgress func(progress float64, downloaded, total int64)
}

// NewProgressReader 创建一个 ProgressReader。
// total 为预期总字节数（0 表示未知，此时不触发回调）；onProgress 为进度回调（可为 nil）。
func NewProgressReader(reader io.Reader, downloaded int64, total int64, onProgress func(float64, int64, int64)) *ProgressReader {
	return &ProgressReader{
		reader:     reader,
		total:      total,
		downloaded: downloaded,
		onProgress: onProgress,
	}
}

// Read 实现 io.Reader。每次读取后更新进度并回调。
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.downloaded += int64(n)
		if pr.total > 0 && pr.onProgress != nil {
			progress := float64(pr.downloaded) / float64(pr.total) * 100
			pr.onProgress(progress, pr.downloaded, pr.total)
		}
	}
	return n, err
}

// Done 标记读取完成，强制设置进度为 100%。
func (pr *ProgressReader) Done() {
	if pr.onProgress != nil && pr.total > 0 {
		pr.onProgress(100, pr.total, pr.total)
	}
}
