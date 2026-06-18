// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package extractor_test

import (
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/download/extractor"
)

func TestCompositeExtractorName(t *testing.T) {
	ex := extractor.NewCompositeExtractor()
	if ex.Name() != "composite" {
		t.Errorf("expected 'composite', got %s", ex.Name())
	}
}

func TestCompositeExtractorMatchAlwaysFalse(t *testing.T) {
	ex := extractor.NewCompositeExtractor()
	if ex.Match(t.Context(), "http://example.com/file") {
		t.Error("CompositeExtractor.Match should always return false")
	}
}

func TestCompositeExtractorNoFiles(t *testing.T) {
	ex := extractor.NewCompositeExtractor()
	err := ex.Extract(t.Context(), &download.Request{
		URL:      "http://example.com/page",
		SavePath: "/tmp/output",
		Metadata: map[string]string{},
	})
	if err == nil {
		t.Error("expected error for no files metadata")
	}
}

func TestCompositeExtractorEmptyFiles(t *testing.T) {
	ex := extractor.NewCompositeExtractor()
	err := ex.Extract(t.Context(), &download.Request{
		URL:      "http://example.com/page",
		SavePath: "/tmp/output",
		Metadata: map[string]string{"files": "[]"},
	})
	if err == nil {
		t.Error("expected error for empty files")
	}
}
