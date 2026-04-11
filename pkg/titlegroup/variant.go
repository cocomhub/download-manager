package titlegroup

import "strings"

// TKTVariantFlags 返回标题的变体标记：是否高画质标签、是否编号后缀 C
func TKTVariantFlags(title string) (hasHQ bool, hasC bool) {
	t := strings.TrimSpace(title)
	if strings.HasPrefix(t, "【") {
		if idx := strings.Index(t, "】"); idx > 0 {
			hasHQ = true
			t = strings.TrimSpace(t[idx+len("】"):])
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

