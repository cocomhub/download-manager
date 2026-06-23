// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"path"
	"strings"
)

// Rule 鎻忚堪涓€涓?URL 璺敱瑙勫垯銆?type Rule struct {
	Pattern   string // URL 妯″紡锛屾敮鎸?path.Match 鎴?*suffix / prefix* / *contains* 璇硶
	Extractor string // 鍖归厤鍚庝娇鐢ㄧ殑 Extractor 鍚嶇О锛堝彲閫夛級
	MinSize   int64  // 鏈€灏忔枃浠跺ぇ灏忥紝0 琛ㄧず涓嶉檺鍒?	MaxSize   int64  // 鏈€澶ф枃浠跺ぇ灏忥紝0 琛ㄧず涓嶉檺鍒?}

// Match 妫€鏌?URL 鏄惁鍖归厤姝よ鍒欍€?func (r *Rule) Match(url string) bool {
	return matchPattern(r.Pattern, url)
}

// matchPattern 鏀寔 path.Match銆佸悗缂€(*.ext)銆佸墠缂€(prefix*)銆佸寘鍚?*sub*) 鍥涚妯″紡銆?func matchPattern(pattern, url string) bool {
	// path.Match glob 鍖归厤
	if ok, err := path.Match(pattern, url); err == nil && ok {
		return true
	}
	// 鍚庣紑鍖归厤 (*.ext)
	if strings.HasPrefix(pattern, "*") && !strings.HasSuffix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(url, suffix)
	}
	// 鍓嶇紑鍖归厤 (prefix*)
	if strings.HasSuffix(pattern, "*") && !strings.HasPrefix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(url, prefix)
	}
	// 鍖呭惈鍖归厤 (*sub*)
	if strings.Count(pattern, "*") == 2 && strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		substr := strings.TrimSuffix(strings.TrimPrefix(pattern, "*"), "*")
		return strings.Contains(url, substr)
	}
	return false
}

// RuleSet 绠＄悊涓€缁勬湁搴忕殑 URL 璺敱瑙勫垯銆?type RuleSet struct {
	rules []*Rule
}

// NewRuleSet 鍒涘缓鍖呭惈鍒濆瑙勫垯鐨?RuleSet銆?func NewRuleSet(rules ...*Rule) *RuleSet {
	rs := &RuleSet{}
	rs.rules = append(rs.rules, rules...)
	return rs
}

// Add 娣诲姞瑙勫垯鍒版湯灏俱€?func (rs *RuleSet) Add(r *Rule) {
	rs.rules = append(rs.rules, r)
}

// Match 鎸夋敞鍐岄『搴忚繑鍥炵涓€涓尮閰嶇殑瑙勫垯锛屾棤鍖归厤杩斿洖 nil銆?func (rs *RuleSet) Match(url string, hint *DownloadHint) *Rule {
	for _, r := range rs.rules {
		if r.Match(url) {
			return r
		}
	}
	return nil
}
