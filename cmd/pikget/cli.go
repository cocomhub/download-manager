// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import "strings"

// kind 是 URL 分发类型。
type kind int

const (
	kindDirect      kind = iota // 普通 http(s) 直链
	kindPikpak                  // PikPak 分享链接（mypikpak/keepshare）
	kindUnsupported             // 磁力/未知 scheme
)

// dispatch 按 URL 返回分发类型：
//   - 普通 http(s) 直链 → kindDirect（走 pkg/download）
//   - mypikpak / keepshare 分享链接 → kindPikpak（走 sproxy hybrid）
//   - magnet / bt / 其它 scheme → kindUnsupported
func dispatch(raw string) kind {
	low := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(low, "magnet:") || strings.HasPrefix(low, "bt:"):
		return kindUnsupported
	case isShareHost(low):
		return kindPikpak
	case strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://"):
		return kindDirect
	default:
		return kindUnsupported
	}
}

// isShareHost 判断是否为 PikPak 分享 URL host（与 sproxy 侧 isShareURL 同源规则）。
func isShareHost(low string) bool {
	return strings.Contains(low, "mypikpak.com/s/") ||
		strings.Contains(low, "mypikpak.net/s/") ||
		strings.Contains(low, "keepshare.org/") ||
		strings.Contains(low, "keepshare.cc/")
}
