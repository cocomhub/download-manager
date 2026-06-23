// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dlcore

import "testing"

func TestIsHlsURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://example.com/stream.m3u8", true},
		{"https://example.com/STREAM.M3U8?token=abc", true},
		{"https://example.com/video.mp4", false},
		{"https://example.com/playlist.m3u8/index.ts", true},
		{"", false},
	}
	for _, c := range cases {
		if got := isHlsURL(c.in); got != c.want {
			t.Fatalf("isHlsURL(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
