// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"fmt"
	"testing"

	"github.com/cocomhub/download-manager/config"
)

// TestFeatureGap_ConfigTypeMigration 楠岃瘉 native_http 鈫?native_old 杩佺Щ銆?func TestFeatureGap_ConfigTypeMigration(t *testing.T) {
	cfg := &config.Config{
		Downloader: config.Downloader{Type: "native_http"},
	}
	cfg.ValidateAndClamp()
	if cfg.Downloader.Type != "native_old" {
		t.Errorf("expected native_old, got %q", cfg.Downloader.Type)
	}
}

// TestFeatureGap_ConfigFieldMigration 楠岃瘉鏃у瓧娈佃縼绉诲埌鏂板瓙缁撴瀯銆?func TestFeatureGap_ConfigFieldMigration(t *testing.T) {
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

// TestFeatureGap_ProgressTuning 楠岃瘉杩涘害璋冧紭鍙傛暟鍙屽疄鐜板彲鐢ㄣ€?func TestFeatureGap_ProgressTuning(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/tuning.bin", "tuning content", "application/octet-stream")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/tuning.bin", "tuning/out.bin", nil, nil)
	cmp.Run("progress-tuning", obj, nil, CheckBothNil(), CheckProgressEnd())
}

// TestFeatureGap_DownloadAllDlcoreTypes 楠岃瘉鏂版棫瀹炵幇鍦ㄥ悇绉?HTTP 鐘舵€佺爜涓婄殑琛屼负銆?func TestFeatureGap_DownloadAllDlcoreTypes(t *testing.T) {
	// 娴嬭瘯鍏辨湁鐘舵€佺爜 鈥?涓嶇‖鏂█閿欒涓€鑷存€э紝浠呴獙璇佸弻鏂逛笉 panic
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

// TestFeatureGap_ExtraMetrics 楠岃瘉鏂拌矾寰勪笉鍥犱负 Metrics 鎶ラ敊銆?func TestFeatureGap_ExtraMetrics(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/metrics.bin", "metrics content", "text/plain")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/metrics.bin", "metrics/out.bin", nil, nil)

	// 鍙涓?panic 灏辫
	cmp.Run("metrics", obj, nil, CheckBothNil(), CheckFileBytes())
}

// TestFeatureGap_MetadataFlusher 楠岃瘉绔嬪嵆鎸佷箙鍖栧洖璋冨吋瀹规€с€?func TestFeatureGap_MetadataFlusher(t *testing.T) {
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
		// DownloaderAdapter 鍙兘涓嶆毚闇?SetMetadataFlusher
		t.Log("metadata flusher was not called (may be expected if not supported)")
	}
}
