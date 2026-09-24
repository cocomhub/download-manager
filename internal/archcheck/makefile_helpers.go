// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package archcheck

// makefile_helpers.go 提供 Makefile 解析辅助（目标配方提取、目标重复检测）。

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// makefileTargetRecipe 返回 Makefile 中 target 的配方（紧跟 target 行、以 Tab 开头的连续行）。
func makefileTargetRecipe(t *testing.T, target string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(moduleRoot(t), "Makefile"))
	if err != nil {
		t.Fatalf("读 Makefile: %v", err)
	}
	var recipe []string
	inTarget := false
	for line := range strings.SplitSeq(string(data), "\n") {
		if !inTarget {
			// 只认「行首即 target:」的定义行，避免命中 .PHONY 之类的引用。
			if strings.HasPrefix(line, target+":") {
				inTarget = true
			}
			continue
		}
		if !strings.HasPrefix(line, "\t") {
			break
		}
		recipe = append(recipe, strings.TrimPrefix(line, "\t"))
	}
	return strings.Join(recipe, "\n")
}

// moduleRoot 返回仓库根目录（本包位于 <root>/internal/archcheck，故上溯两级）。
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位 archcheck 源文件路径")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}
