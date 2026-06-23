// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package titlegroup

import (
	"regexp"
	"strings"
)

var tktRegex = regexp.MustCompile(`([A-Z]+-\d{2,5})(?:C)?\b`)

// TKTGroupNameFromTitle 浠庢爣棰樿В鏋?tktube 鍒嗙粍鍚嶃€?//
// 瑙勫垯锛?// 1) 鍘婚櫎寮€澶寸殑鍏ㄨ鎷彿鏍囩锛屼緥濡?"銆愰珮鐢昏川銆?锛?// 2) 鍖归厤姝ｅ垯 `([A-Z]+-\d{2,5})(?:C)?\b`锛屾崟鑾风粍1浣滀负缁撴灉锛堝幓闄ゅ彲閫夊悗缂€ C锛夛紱
// 3) 缁撴灉缁熶竴杞负澶у啓锛涗笉鍖归厤鍒欒繑鍥炵┖涓层€?//
// 绀轰緥锛歴
// - "CLUB-100"        -> "CLUB-100"
// - "CLUB-100C"       -> "CLUB-100"
// - "銆愰珮鐢昏川銆慍LUB-100"  -> "CLUB-100"
// - "銆愰珮鐢昏川銆慍LUB-100C" -> "CLUB-100"
func TKTGroupNameFromTitle(title string) string {
	t := strings.TrimSpace(title)
	if strings.HasPrefix(t, "銆?) {
		if idx := strings.Index(t, "銆?); idx > 0 {
			t = strings.TrimSpace(t[idx+len("銆?):])
		}
	}
	t = strings.ToUpper(t)
	m := tktRegex.FindStringSubmatch(t)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// TKTContentGroupKey returns a non-empty tktube content group key.
// Parsed canonical groups keep their legal group name, otherwise the key falls
// back to a per-object unknown bucket so unknown titles are never merged.
func TKTContentGroupKey(title, url string) string {
	if group := TKTGroupNameFromTitle(title); group != "" {
		return group
	}

	title = strings.TrimSpace(title)
	if title != "" {
		return "unknown+" + title
	}

	url = strings.TrimSpace(url)
	if url != "" {
		return "unknown+" + url
	}

	return "unknown"
}
