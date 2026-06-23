// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

func TestRuleSuffixMatch(t *testing.T) {
	r := &download.Rule{Pattern: "*.m3u8"}
	if !r.Match("http://example.com/stream.m3u8") {
		t.Error("should match *.m3u8")
	}
	if r.Match("http://example.com/file.mp4") {
		t.Error("should NOT match .mp4")
	}
}

func TestRulePrefixMatch(t *testing.T) {
	r := &download.Rule{Pattern: "https://secure*"}
	if !r.Match("https://secure.example.com/file") {
		t.Error("should match https://secure prefix")
	}
	if r.Match("http://example.com/file") {
		t.Error("should NOT match http://")
	}
}

func TestRuleContainsMatch(t *testing.T) {
	r := &download.Rule{Pattern: "*large*"}
	if !r.Match("http://example.com/large-file.zip") {
		t.Error("should match *large*")
	}
	if r.Match("http://example.com/small-file.zip") {
		t.Error("should NOT match non-large")
	}
}

func TestRuleExactMatch(t *testing.T) {
	r := &download.Rule{Pattern: "http://example.com/file.zip"}
	if !r.Match("http://example.com/file.zip") {
		t.Error("should match exact URL")
	}
}

func TestRuleSetNoMatch(t *testing.T) {
	rs := download.NewRuleSet()
	r := rs.Match("http://example.com/file", nil)
	if r != nil {
		t.Error("expected nil for empty ruleset")
	}
}

func TestRuleSetOrderedMatch(t *testing.T) {
	rs := download.NewRuleSet(
		&download.Rule{Pattern: "*.mp4", Extractor: "ffmpeg"},
		&download.Rule{Pattern: "*.m3u8", Extractor: "hls"},
	)
	r := rs.Match("http://example.com/stream.m3u8", nil)
	if r == nil || r.Extractor != "hls" {
		t.Errorf("expected hls extractor, got %v", r)
	}
}

func TestRuleSetAddRule(t *testing.T) {
	rs := download.NewRuleSet()
	rs.Add(&download.Rule{Pattern: "*.mp4"})
	if rs.Match("http://example.com/file.mp4", nil) == nil {
		t.Error("expected match after Add")
	}
}
