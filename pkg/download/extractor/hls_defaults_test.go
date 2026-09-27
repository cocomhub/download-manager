// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"slices"
	"strings"
	"testing"
)

// TestDefaultFFmpegArgs_StreamingLowMemory 验证默认 ffmpeg 转封装参数低内存：
// 1. 不含 +faststart（faststart 缓存 moov 到内存，1.4GB 视频吃 ~800MB）
// 2. 含 -max_muxing_queue_size（限制 muxer 队列深度）
// 3. 保留 -c copy（不重编码视频，零拷贝）
func TestDefaultFFmpegArgs_StreamingLowMemory(t *testing.T) {
	joined := strings.Join(defaultFFmpegArgs, " ")
	for _, forbidden := range []string{"+faststart"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("default ffmpeg args contain %q (faststart caches moov in memory): %v", forbidden, defaultFFmpegArgs)
		}
	}
	for _, required := range []string{"-max_muxing_queue_size", "-c", "copy", "-bsf:a", "aac_adtstoasc"} {
		if !slices.Contains(defaultFFmpegArgs, required) {
			t.Errorf("default ffmpeg args missing %q: %v", required, defaultFFmpegArgs)
		}
	}
}

// TestNewHLSExtractor_DefaultArgsNoFaststart 验证 NewHLSExtractor 默认参数不含 faststart。
func TestNewHLSExtractor_DefaultArgsNoFaststart(t *testing.T) {
	ex := NewHLSExtractor()
	joined := strings.Join(ex.ffmpegArgs, " ")
	if strings.Contains(joined, "+faststart") {
		t.Errorf("NewHLSExtractor default args contain +faststart: %v", ex.ffmpegArgs)
	}
	if !slices.Contains(ex.ffmpegArgs, "-max_muxing_queue_size") {
		t.Errorf("NewHLSExtractor default args missing -max_muxing_queue_size: %v", ex.ffmpegArgs)
	}
}
