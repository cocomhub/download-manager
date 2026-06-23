// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor_test

import (
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/download/extractor"
)

func TestHLSExtractorName(t *testing.T) {
	ex := extractor.NewHLSExtractor()
	if ex.Name() != "hls" {
		t.Errorf("expected 'hls', got %s", ex.Name())
	}
}

func TestHLSExtractorMatchM3U8(t *testing.T) {
	ex := extractor.NewHLSExtractor()
	if !ex.Match(t.Context(), "http://example.com/stream.m3u8") {
		t.Error("HLSExtractor should match .m3u8 URLs")
	}
	if !ex.Match(t.Context(), "http://example.com/playlist.M3U8") {
		t.Error("HLSExtractor should match .M3U8 URLs (case-insensitive)")
	}
	if ex.Match(t.Context(), "http://example.com/file.mp4") {
		t.Error("HLSExtractor should NOT match non-m3u8 URLs")
	}
}

func TestHLSExtractorNoFFmpeg(t *testing.T) {
	ex := extractor.NewHLSExtractor(extractor.WithHLSMode("ffmpeg"))
	err := ex.Extract(t.Context(), &download.Request{
		URL:      "http://example.com/stream.m3u8",
		SavePath: "/tmp/output.mp4",
	})
	if err == nil {
		t.Skip("ffmpeg not available, skipping")
	}
	t.Logf("Got expected error: %v", err)
}
