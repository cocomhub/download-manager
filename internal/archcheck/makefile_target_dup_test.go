// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package archcheck

// makefile_target_dup_test.go 门禁：Makefile 中同一目标不得被定义两次
// （对齐 sproxy R16）。
//
// GNU make 对重复目标**不报错**，只用后一份覆盖前一份——本门禁一刀切禁止重复定义。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakefileNoDuplicateTargets(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(moduleRoot(t), "Makefile"))
	if err != nil {
		t.Fatalf("读取 Makefile: %v", err)
	}

	seen := map[string]int{}
	var order []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, " ") ||
			strings.HasPrefix(line, "#") || strings.HasPrefix(line, ".") {
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		if eq := strings.Index(line, "="); eq >= 0 && (eq < colon || eq == colon+1) {
			continue // 变量赋值（= / := / ?= / +=）
		}
		for name := range strings.FieldsSeq(line[:colon]) {
			if seen[name] == 0 {
				order = append(order, name)
			}
			seen[name]++
		}
	}

	// 正探针：解析面必须非平凡（防「一个目标都没解析到 ⇒ 断言空转」）。
	if len(seen) < 30 {
		t.Fatalf("只解析到 %d 个 Makefile 目标（判据失效，需更新本测试）", len(seen))
	}

	var dups []string
	for _, name := range order {
		if seen[name] > 1 {
			dups = append(dups, name)
		}
	}
	if len(dups) > 0 {
		t.Fatalf("Makefile 存在重复定义的目标：%s。GNU make 不报错，只用**后一份**覆盖前一份。",
			strings.Join(dups, ", "))
	}
}
