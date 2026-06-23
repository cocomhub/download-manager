// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package titlegroup

import "strings"

// TKTVariantFlags 杩斿洖鏍囬鐨勫彉浣撴爣璁帮細鏄惁楂樼敾璐ㄦ爣绛俱€佹槸鍚︾紪鍙峰悗缂€ C
func TKTVariantFlags(title string) (hasHQ bool, hasC bool) {
	t := strings.TrimSpace(title)
	if strings.HasPrefix(t, "銆?) {
		if idx := strings.Index(t, "銆?); idx > 0 {
			hasHQ = true
			t = strings.TrimSpace(t[idx+len("銆?):])
		}
	}
	up := strings.ToUpper(t)
	if m := tktRegex.FindStringSubmatch(up); len(m) > 0 {
		// Check if immediately followed by C (case-insensitive)
		full := tktRegex.FindString(up)
		if strings.HasSuffix(full, "C") {
			hasC = true
		}
	}
	return
}
