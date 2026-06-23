// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dlcore

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	m3u8dlib "github.com/cocomhub/download-manager/pkg/m3u8d"
)

// m3u8dHandler 浣跨敤 m3u8d 搴撲笅杞?HLS (m3u8) 娴併€?type m3u8dHandler struct{}

func (h *m3u8dHandler) Match(url string) bool {
	return strings.Contains(strings.ToLower(url), ".m3u8")
}

func (h *m3u8dHandler) Name() string { return "m3u8d" }

func (h *m3u8dHandler) Download(ctx context.Context, req *Request) error {
	cfg := &m3u8dlib.DownloadConfig{
		InputURL:    req.URL,
		OutputFile:  req.SavePath,
		UserAgent:   "Mozilla/5.0",
		Headers:     req.Headers,
		Concurrency: 4,
		MaxRetries:  3,
		KeepFiles:   false,
		WorkDir:     filepath.Dir(req.SavePath) + "_m3u8d_work",
		FFmpegArgs:  []string{"-c", "copy", "-bsf:a", "aac_adtstoasc", "-movflags", "+faststart", "-f", "mp4"},
		Timeout:     0,
		Verbose:     false,
	}

	dl, err := m3u8dlib.NewM3U8Downloader(cfg)
	if err != nil {
		return fmt.Errorf("m3u8d: init failed: %w", err)
	}

	localM3U8, err := dl.DownloadAll(ctx)
	if err != nil {
		return fmt.Errorf("m3u8d: download failed: %w", err)
	}

	if err := dl.ConvertToMP4(ctx, localM3U8); err != nil {
		return fmt.Errorf("m3u8d: convert failed: %w", err)
	}

	if err := dl.Cleanup(); err != nil {
		return fmt.Errorf("m3u8d: cleanup failed: %w", err)
	}

	if req.Metadata != nil {
		req.Metadata["status"] = StatusCompleted
	}
	if req.TrackProgress && req.OnProgress != nil {
		req.OnProgress(100, 0, 0)
	}
	return nil
}

// init 鑷姩娉ㄥ唽 m3u8d handler锛屼紭鍏堢骇楂樹簬 ffmpeg handler
func init() {
	RegisterHandler("m3u8d", &m3u8dHandler{})
}
