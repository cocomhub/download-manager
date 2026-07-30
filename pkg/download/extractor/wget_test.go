// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor_test

import (
	"context"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/download/extractor"
)

// mockSelector 实现 download.Selector 接口，用于测试。
var _ download.Selector = (*mockSelector)(nil)

type mockSelector struct{}

func (m *mockSelector) MatchExtractor(_ context.Context, _ string, _ *download.DownloadHint) download.Extractor {
	return nil
}
func (m *mockSelector) SelectProxy(_ context.Context, _ string, _ *download.DownloadHint) (string, error) {
	return "", nil
}

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
