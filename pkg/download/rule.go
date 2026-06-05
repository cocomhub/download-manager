// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"path"
	"strings"
)

// Rule 描述一个 URL 路由规则。
type Rule struct {
	Pattern   string // URL 模式，支持 path.Match 或 *suffix / prefix* / *contains* 语法
	Extractor string // 匹配后使用的 Extractor 名称（可选）
	MinSize   int64  // 最小文件大小，0 表示不限制
	MaxSize   int64  // 最大文件大小，0 表示不限制
}

// Match 检查 URL 是否匹配此规则。
func (r *Rule) Match(url string) bool {
	return matchPattern(r.Pattern, url)
}

// matchPattern 支持 path.Match、后缀(*.ext)、前缀(prefix*)、包含(*sub*) 四种模式。
func matchPattern(pattern, url string) bool {
	// path.Match glob 匹配
	if ok, err := path.Match(pattern, url); err == nil && ok {
		return true
	}
	// 后缀匹配 (*.ext)
	if strings.HasPrefix(pattern, "*") && !strings.HasSuffix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(url, suffix)
	}
	// 前缀匹配 (prefix*)
	if strings.HasSuffix(pattern, "*") && !strings.HasPrefix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(url, prefix)
	}
	// 包含匹配 (*sub*)
	if strings.Count(pattern, "*") == 2 && strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		substr := strings.TrimSuffix(strings.TrimPrefix(pattern, "*"), "*")
		return strings.Contains(url, substr)
	}
	return false
}

// RuleSet 管理一组有序的 URL 路由规则。
type RuleSet struct {
	rules []*Rule
}

// NewRuleSet 创建包含初始规则的 RuleSet。
func NewRuleSet(rules ...*Rule) *RuleSet {
	rs := &RuleSet{}
	rs.rules = append(rs.rules, rules...)
	return rs
}

// Add 添加规则到末尾。
func (rs *RuleSet) Add(r *Rule) {
	rs.rules = append(rs.rules, r)
}

// Match 按注册顺序返回第一个匹配的规则，无匹配返回 nil。
func (rs *RuleSet) Match(url string, hint *DownloadHint) *Rule {
	for _, r := range rs.rules {
		if r.Match(url) {
			return r
		}
	}
	return nil
}
