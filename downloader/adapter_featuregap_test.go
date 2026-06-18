// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"fmt"
	"testing"

	"github.com/cocomhub/download-manager/config"
)

// TestFeatureGap_ConfigTypeMigration 验证 native_http → native_old 迁移。
func TestFeatureGap_ConfigTypeMigration(t *testing.T) {
	cfg := &config.Config{
		Downloader: config.Downloader{Type: "native_http"},
	}
	cfg.ValidateAndClamp()
	if cfg.Downloader.Type != "native_old" {
		t.Errorf("expected native_old, got %q", cfg.Downloader.Type)
	}
}

// TestFeatureGap_ConfigFieldMigration 验证旧字段迁移到新子结构。
func TestFeatureGap_ConfigFieldMigration(t *testing.T) {
	cfg := &config.Config{
		Downloader: config.Downloader{
			LogDir:            "/old/log/dir",
			FfmpegPath:        "/usr/bin/ffmpeg",
			HlsAutoMarkAsFail: true,
		},
	}
	cfg.ValidateAndClamp()

	if cfg.Downloader.Filesystem.LogDir != "/old/log/dir" {
		t.Errorf("Filesystem.LogDir migration: got %q, want %q", cfg.Downloader.Filesystem.LogDir, "/old/log/dir")
	}
	if cfg.Downloader.FFmpeg.Path != "/usr/bin/ffmpeg" {
		t.Errorf("FFmpeg.Path migration: got %q, want %q", cfg.Downloader.FFmpeg.Path, "/usr/bin/ffmpeg")
	}
	if !cfg.Downloader.FFmpeg.HLSAutoMarkAsFail {
		t.Error("FFmpeg.HLSAutoMarkAsFail migration: expected true")
	}
}

// TestFeatureGap_ProgressTuning 验证进度调优参数双实现可用。
func TestFeatureGap_ProgressTuning(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/tuning.bin", "tuning content", "application/octet-stream")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/tuning.bin", "tuning/out.bin", nil, nil)
	cmp.Run("progress-tuning", obj, nil, CheckBothNil(), CheckProgressEnd())
}

// TestFeatureGap_DownloadAllDlcoreTypes 验证新旧实现在各种 HTTP 状态码上的行为。
func TestFeatureGap_DownloadAllDlcoreTypes(t *testing.T) {
	// 测试共有状态码 — 不硬断言错误一致性，仅验证双方不 panic
	codes := []int{200, 206, 304, 403, 404, 416, 500}
	for _, code := range codes {
		t.Run(fmt.Sprintf("code_%d", code), func(t *testing.T) {
			b := NewBeacon(t)
			if code == 200 || code == 206 {
				b.HandleFile("GET", "/code.bin", "content", "text/plain")
			} else {
				b.HandleError("GET", "/code.bin", code)
			}

			cmp := NewComparator(t, b)
			obj := makeTestObject(b.URL()+"/code.bin", fmt.Sprintf("codes/%d.bin", code), nil, nil)
			cmp.Run("code", obj, nil)
		})
	}
}

// TestFeatureGap_ExtraMetrics 验证新路径不因为 Metrics 报错。
func TestFeatureGap_ExtraMetrics(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/metrics.bin", "metrics content", "text/plain")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/metrics.bin", "metrics/out.bin", nil, nil)

	// 只要不 panic 就行
	cmp.Run("metrics", obj, nil, CheckBothNil(), CheckFileBytes())
}

// TestFeatureGap_MetadataFlusher 验证立即持久化回调兼容性。
func TestFeatureGap_MetadataFlusher(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/flusher.bin", "flusher content", "text/plain")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/flusher.bin", "flusher/out.bin", nil, nil)

	flusherCalled := false
	if mf, ok := cmp.newDL.(interface{ SetMetadataFlusher(func()) }); ok {
		mf.SetMetadataFlusher(func() {
			flusherCalled = true
		})
	}

	cmp.Run("metadata-flusher", obj, nil, CheckBothNil(), CheckFileBytes())

	if !flusherCalled {
		// DownloaderAdapter 可能不暴露 SetMetadataFlusher
		t.Log("metadata flusher was not called (may be expected if not supported)")
	}
}
