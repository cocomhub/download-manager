// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package archcheck

// notest_gate_test.go 是「**`make notest` 门禁自身可用性**」的守卫。
//
// 背景（#81 已修复核心缺陷）：check-test-files.sh 曾被两处 bug 废掉——
// ① Makefile 空参调用导致脚本零次迭代直接打印 OK（门禁从未生效）；
// ② 忽略清单用 find -not -path 反向实现，.notestignore 从未生效。
// 本测试钉住「门禁必须接线且 fail-closed」，防止退化。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotestGate_WiredAndFailsClosed(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	read := func(rel string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("读 %s: %v", rel, err)
		}
		return string(data)
	}

	// 1) 调用方必须把包列表传进去（否则脚本空转）
	recipe := makefileTargetRecipe(t, "notest")
	if !strings.Contains(recipe, "check-test-files.sh") {
		t.Fatalf("notest 应以 scripts/check-test-files.sh 调用:\n%s", recipe)
	}
	if !strings.Contains(recipe, "$(ALL_PKGS)") && !strings.Contains(recipe, "go list") {
		t.Errorf("notest 必须把包列表作为参数传给脚本（不带参数 = 门禁空转）：\n%s", recipe)
	}

	// 2) 脚本必须 fail-closed：没有参数时不能打印 OK
	script := read("scripts/check-test-files.sh")
	if !strings.Contains(script, "$# -eq 0") || !strings.Contains(script, "exit 1") {
		t.Error("check-test-files.sh 必须对「空参数」显式失败（门禁不得空转）")
	}
	// 3) 忽略清单必须是正向匹配（历史缺陷：find -not -path 反向实现）
	if !strings.Contains(script, "IGNORE_PATTERNS") {
		t.Error("check-test-files.sh 的忽略清单应按 glob 正向匹配（原实现反向，导致 .notestignore 失效）")
	}

	// 4) CI 必须真的调用它（只修本地目标 = 纸面门禁）
	// 用「行内精确匹配」而非子串：'make notest' 是 'make notest-NEVER' 的子串，
	// 子串匹配会被变异绕过。
	ci := read(".github/workflows/ci.yml")
	found := false
	for line := range strings.SplitSeq(ci, "\n") {
		trimmed := strings.TrimSpace(line)
		// YAML 行形如 "run: make notest"；精确校验「以 make notest 结尾且非 make notest-*」。
		if strings.HasSuffix(trimmed, "make notest") && !strings.HasSuffix(trimmed, "make notest-") {
			found = true
			break
		}
	}
	if !found {
		t.Error("CI 未调用 make notest（门禁未接线 ⇒ 永不生效）")
	}
}
