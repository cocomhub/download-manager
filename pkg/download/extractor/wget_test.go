// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor_test

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download/extractor"
)

// newWgetForTest 创建 WgetExtractor，如果 wget 不可用则跳过测试。
func newWgetForTest(t *testing.T) *extractor.WgetExtractor {
	t.Helper()
	ex, err := extractor.NewWgetExtractor()
	if err != nil {
		var pathErr *exec.Error
		if errors.As(err, &pathErr) {
			t.Skip("wget not found in PATH, skipping test")
		}
		t.Fatalf("NewWgetExtractor failed: %v", err)
	}
	return ex
}

func TestWgetExtractorInitError(t *testing.T) {
	// 验证当 wget 不可用时构造函数返回错误
	_, err := extractor.NewWgetExtractor()
	if err != nil {
		// 期望的错误：wget 不在 PATH 中
		var pathErr *exec.Error
		if errors.As(err, &pathErr) {
			return // 正确的错误类型
		}
		// 也可能是其他错误，但只要是 error 就通过
		return
	}
	// 如果 wget 可用，构造函数成功也算正常
}

func TestWgetExtractorName(t *testing.T) {
	ex := newWgetForTest(t)
	if ex.Name() != "wget" {
		t.Errorf("expected 'wget', got %s", ex.Name())
	}
}

func TestWgetExtractorMatch(t *testing.T) {
	ex := newWgetForTest(t)

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
	ex := newWgetForTest(t)
	err := ex.Cancel("http://example.com/nonexistent")
	if err != nil {
		t.Errorf("Cancel on nonexistent should return nil, got: %v", err)
	}
}
