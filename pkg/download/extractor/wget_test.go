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

	// WgetExtractor 不参与自动 URL 匹配，仅通过 hint.Extractor 显式选择
	if ex.Match(t.Context(), "http://example.com/file.zip") {
		t.Error("WgetExtractor.Match should always return false")
	}
	if ex.Match(t.Context(), "https://cdn.example.com/video.mp4") {
		t.Error("WgetExtractor.Match should always return false")
	}
	if ex.Match(t.Context(), "http://cdn.example.com/stream.m3u8") {
		t.Error("WgetExtractor.Match should always return false")
	}
}

func TestWgetExtractorCancel(t *testing.T) {
	ex := extractor.NewWgetExtractor()
	err := ex.Cancel("http://example.com/nonexistent")
	if err != nil {
		t.Errorf("Cancel on nonexistent should return nil, got: %v", err)
	}
}
