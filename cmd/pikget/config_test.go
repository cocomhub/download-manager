// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// TestDefaultConfig_AnonRatioAutoAndJournalOn 钉住默认配置语义（覆盖审计 2026-10-28）：
// share_ratio=0（auto：匿名区上限由 sproxy 探测决定，不再固定 0.5）；
// 进度 WAL 默认启用（disable_progress_journal=false）。
func TestDefaultConfig_AnonRatioAutoAndJournalOn(t *testing.T) {
	t.Parallel()
	cfg := defaultConfig()
	if cfg.Downloader.Pikpak.ShareRatio != 0 {
		t.Fatalf("默认为 auto（0），got %v", cfg.Downloader.Pikpak.ShareRatio)
	}
	if cfg.Downloader.Pikpak.DisableProgressJournal {
		t.Fatal("进度 WAL 默认应启用（disable=false）")
	}
	if cfg.Downloader.Pikpak.ChunkSize <= 0 {
		t.Fatalf("chunk size 默认值异常: %d", cfg.Downloader.Pikpak.ChunkSize)
	}
}
