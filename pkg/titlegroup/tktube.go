package titlegroup

import (
	"regexp"
	"strings"
)

var tktRegex = regexp.MustCompile(`([A-Z]+-\d{2,5})(?:C)?\b`)

// TKTGroupNameFromTitle 从标题解析 tktube 分组名。
//
// 规则：
// 1) 去除开头的全角括号标签，例如 "【高画质】"；
// 2) 匹配正则 `([A-Z]+-\d{2,5})(?:C)?\b`，捕获组1作为结果（去除可选后缀 C）；
// 3) 结果统一转为大写；不匹配则返回空串。
//
// 示例：s
// - "CLUB-100"        -> "CLUB-100"
// - "CLUB-100C"       -> "CLUB-100"
// - "【高画质】CLUB-100"  -> "CLUB-100"
// - "【高画质】CLUB-100C" -> "CLUB-100"
func TKTGroupNameFromTitle(title string) string {
	t := strings.TrimSpace(title)
	if strings.HasPrefix(t, "【") {
		if idx := strings.Index(t, "】"); idx > 0 {
			t = strings.TrimSpace(t[idx+len("】"):])
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
