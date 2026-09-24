// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package archcheck

// release_policy_test.go 是「**CHANGELOG 单一事实源不得被回退成手工维护**」的门禁
// （对齐 sproxy R12）。
//
// 判据（结构性，不锁死正文）：
//  1. release-please-config.json 存在，且根包 `.` 声明 changelog-path: CHANGELOG.md、
//     include-v-in-tag: true；
//  2. pr-title.yml 存在（PR 标题 Conventional Commits 校验）；
//  3. CHANGELOG.md 不得含 `## [Unreleased]` 段（release-please 不消费它，内容会静默丢失）；
//  4. changelog-sections 必须含 remove → Removed（删除对外 API 用提交类型表达）。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleasePleaseIsChangelogSingleSource(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)

	cfgPath := filepath.Join(root, "release-please-config.json")
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("release-please 配置缺失（%s）: %v", cfgPath, err)
	}

	var cfg struct {
		IncludeVInTag     bool `json:"include-v-in-tag"`
		ChangelogSections []struct {
			Type    string `json:"type"`
			Section string `json:"section"`
		} `json:"changelog-sections"`
		Packages map[string]struct {
			ChangelogPath string `json:"changelog-path"`
		} `json:"packages"`
	}
	if err = json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("解析 %s 失败: %v", "release-please-config.json", err)
	}
	pkg, ok := cfg.Packages["."]
	if !ok {
		t.Fatal("release-please-config.json 必须声明根包 \".\"")
	}
	if pkg.ChangelogPath != "CHANGELOG.md" {
		t.Fatalf("根包 changelog-path = %q，必须为 CHANGELOG.md（单一事实源）", pkg.ChangelogPath)
	}
	if !cfg.IncludeVInTag {
		t.Fatal("include-v-in-tag 必须为 true（tag 形如 vX.Y.Z，与 GoReleaser 触发一致）")
	}

	// changelog-sections 必须含 remove → Removed
	hasRemove := false
	for _, s := range cfg.ChangelogSections {
		if s.Type == "remove" && s.Section == "Removed" {
			hasRemove = true
		}
	}
	if !hasRemove {
		t.Error("changelog-sections 必须含 remove → Removed（删除对外 API 用提交类型表达）")
	}

	// pr-title.yml 必须存在
	if _, err := os.Stat(filepath.Join(root, ".github", "workflows", "pr-title.yml")); err != nil {
		t.Error("pr-title.yml 缺失（PR 标题 Conventional Commits 校验门禁）")
	}

	// CHANGELOG.md 不得含 [Unreleased]
	cl, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("CHANGELOG.md 不可读: %v", err)
	}
	if strings.Contains(string(cl), "## [Unreleased]") {
		t.Error("CHANGELOG.md 不得含 [Unreleased] 段（release-please 不消费它，内容会静默丢失）")
	}
}
