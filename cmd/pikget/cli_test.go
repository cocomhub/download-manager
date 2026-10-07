// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestDispatch(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want kind
	}{
		{name: "http 直链", url: "http://example.com/a.mp4", want: kindDirect},
		{name: "https 直链带查询", url: "https://cdn.example.com/v.mp4?token=abc", want: kindDirect},
		{name: "大写 http", url: "HTTPS://EXAMPLE.COM/x.zip", want: kindDirect},
		{name: "mypikpak.com 分享", url: "https://mypikpak.com/s/V1bzETjSE3NwdS", want: kindPikpak},
		{name: "mypikpak.net 分享带 token", url: "https://mypikpak.net/s/abc/XYZ123", want: kindPikpak},
		{name: "keepshare.org 分享", url: "https://keepshare.org/V1bzETjSE3NwdS", want: kindPikpak},
		{name: "keepshare.cc 分享", url: "https://keepshare.cc/abc/def", want: kindPikpak},
		{name: "磁力 magnet", url: "magnet:?xt=urn:btih:ABC", want: kindUnsupported},
		{name: "磁力 bt", url: "bt:ABC", want: kindUnsupported},
		{name: "ftp scheme", url: "ftp://example.com/a.bin", want: kindUnsupported},
		{name: "空串", url: "", want: kindUnsupported},
		{name: "无 scheme 路径", url: "/a/b/c.mp4", want: kindUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()
			if got := dispatch(tt.url); got != tt.want {
				t.Errorf("dispatch(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

// TestDispatchShareBeforeScheme 确保分享 host 判定先于 https 前缀判定
// （分享 URL 也是 https，若顺序颠倒会被误判为直链）。
func TestDispatchShareBeforeScheme(t *testing.T) {
	urls := []string{
		"https://mypikpak.com/s/ABCDEF",
		"https://keepshare.org/x",
	}
	for _, u := range urls {
		t.Helper()
		if got := dispatch(u); got != kindPikpak {
			t.Errorf("dispatch(%q) = %v, want kindPikpak", u, got)
		}
	}
}
