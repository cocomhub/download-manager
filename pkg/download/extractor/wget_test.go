// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor_test

import (
	"testing"

	"github.com/cocomhub/download-manager/pkg/download/extractor"
)

func TestWgetExtractorName(t *testing.T) {
	ex := extractor.NewWgetExtractor()
	if ex.Name() != "wget" {
		t.Errorf("expected 'wget', got %s", ex.Name())
	}
}

func TestWgetExtractorMatch(t *testing.T) {
	ex := extractor.NewWgetExtractor()

	// WgetExtractor 应匹配普通 HTTP URL
	if !ex.Match(t.Context(), "http://example.com/file.zip") {
		t.Error("WgetExtractor should match non-m3u8 URL")
	}
	if !ex.Match(t.Context(), "https://cdn.example.com/video.mp4") {
		t.Error("WgetExtractor should match https URL")
	}

	// WgetExtractor 不应匹配 .m3u8 URL（由 HLSExtractor 处理）
	if ex.Match(t.Context(), "http://cdn.example.com/stream.m3u8") {
		t.Error("WgetExtractor should NOT match .m3u8 URL")
	}
	if ex.Match(t.Context(), "https://cdn.example.com/playlist.M3U8") {
		t.Error("WgetExtractor should NOT match .M3U8 URL (case-insensitive)")
	}
}

func TestWgetExtractorCancel(t *testing.T) {
	ex := extractor.NewWgetExtractor()
	err := ex.Cancel("http://example.com/nonexistent")
	if err != nil {
		t.Errorf("Cancel on nonexistent should return nil, got: %v", err)
	}
}

func TestWgetExtractorSetSelector(t *testing.T) {
	ex := extractor.NewWgetExtractor()
	ex.SetSelector(nil)
}

func TestWgetExtractorSetTransport(t *testing.T) {
	ex := extractor.NewWgetExtractor()
	ex.SetTransport(nil)
}
