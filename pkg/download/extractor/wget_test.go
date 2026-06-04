// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor_test

import (
	"context"
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
	if !ex.Match(context.Background(), "http://example.com/file.zip") {
		t.Error("WgetExtractor should match any URL")
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