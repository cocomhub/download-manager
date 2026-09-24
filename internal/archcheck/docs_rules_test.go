// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package archcheck

// docs_rules_test.go 是「**协作与实施规则文档不得腐烂**」的门禁（对齐 sproxy R9）。
//
// 判据（语义化，不锁死正文）：
//  1. AGENTS.md 与 CLAUDE.md 必须都存在（两镜像一致）；
//  2. AGENTS.md 必须包含关键硬规则锚点（如「中文回复」「TDD」「CI」）——防被清空成空壳；
//  3. 两文件的关键规则镜像一致（都提到 TDD）。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDocsRulesAgentsClaudeMirror 断言 AGENTS.md 与 CLAUDE.md 都存在且含关键锚点。
func TestDocsRulesAgentsClaudeMirror(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)

	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md 缺失: %v", err)
	}
	claude, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md 缺失: %v", err)
	}
	agentsStr := string(agents)
	claudeStr := string(claude)

	// 正探针：两文件都必须有实质内容（防被清空成标题）。
	if len(agentsStr) < 500 {
		t.Fatalf("AGENTS.md 过短（%d 字节 < 500）：疑似被清空成占位", len(agentsStr))
	}
	if len(claudeStr) < 500 {
		t.Fatalf("CLAUDE.md 过短（%d 字节 < 500）：疑似被清空成占位", len(claudeStr))
	}

	// 关键锚点（硬规则不能丢）。
	anchors := []string{"TDD", "中文", "CI"}
	for _, a := range anchors {
		if !strings.Contains(agentsStr, a) {
			t.Errorf("AGENTS.md 缺少关键锚点 %q（规则文档不得腐烂）", a)
		}
		if !strings.Contains(claudeStr, a) {
			t.Errorf("CLAUDE.md 缺少关键锚点 %q（规则文档不得腐烂）", a)
		}
	}
}
