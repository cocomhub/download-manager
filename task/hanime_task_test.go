// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractVideoIDFromURL(t *testing.T) {
	id := extractVideoIDFromURL("https://hanime1.me/watch?v=404480")
	if id != "404480" {
		t.Fatalf("expect 404480, got %s", id)
	}
}

func TestUrlEncodeGenre(t *testing.T) {
	enc := urlEncodeGenre("Motion Anime")
	if !(enc == "Motion+Anime" || enc == "Motion%20Anime") {
		t.Fatalf("unexpected encoding: %s", enc)
	}
}

func TestParseVideoPageHTML(t *testing.T) {
	path := filepath.Join("/Users/libing/Documents/trae_projects/download-manager/web/hanime", "watch?v=404480.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	info, err := parseVideoPageHTML("https://hanime1.me/watch?v=404480", string(data))
	if err != nil {
		t.Fatalf("parse video html error: %v", err)
	}
	if strings.TrimSpace(info.title) == "" {
		t.Fatalf("title empty")
	}
}

func TestParseHomePage(t *testing.T) {
	path := filepath.Join("/Users/libing/Documents/trae_projects/download-manager/web/hanime", "search?genre=Motion Anime.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	items, err := (&HanimeTask{}).parseHomePage(string(data))
	if err != nil {
		t.Fatalf("parse home error: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("no items parsed")
	}
}

func TestNamingFormat(t *testing.T) {
	title := "Sample Title"
	id := "404480"
	base := strings.TrimSpace("[" + id + "] " + title)
	if base != "[404480] Sample Title" {
		t.Fatalf("unexpected base name: %s", base)
	}
}
