// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package archcheck

// dead_symbols_test.go 是「**已确认删除的死代码不得复活**」的墓碑门禁
// （对齐 sproxy R11）。
//
// 判据（结构性，不做语义猜测）：以下符号不得以**词边界**形式出现在任何**非测试**
// 源码中。允许出现在 `_test.go` 中（例如把旧调用点改写为规范入口的对照断言）。

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// deadSymbols 是历史审计确认删除的符号（逐条附删除依据）。
var deadSymbols = []string{
	// 旧下载引擎的 MD5 校验函数（pkg/dlcore 时代，PR #71 退役）。
	"computeFileMD5",
	// 旧下载引擎的图片 URL 探测函数（pkg/dlcore 时代）。
	"isImageURL",
	// dlcore-only 对比测试专用 runner（Comparator 套件已随 dlcore 退役）。
	"DlcoreOnlyRun",
	// 旧 m3u8 下载引擎类型（pkg/m3u8d，P4-2 已收敛删除）。
	"NewM3U8Downloader",
}

// scanDeadSymbol 在 root 下遍历非测试 .go 文件，以**词边界**匹配 sym，
// 返回 "相对路径:行号: 行内容" 形式的命中列表。
func scanDeadSymbol(root, sym string) ([]string, error) {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(sym) + `\b`)
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// 跳过隐藏目录、node_modules、.git
			if strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if re.MatchString(line) {
				rel, _ := filepath.Rel(root, path)
				hits = append(hits, fmt.Sprintf("%s:%d: %s", rel, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hits, nil
}

// TestDeadSymbolsDoNotResurrect 断言墓碑清单中的符号零命中。
func TestDeadSymbolsDoNotResurrect(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	for _, sym := range deadSymbols {
		hits, err := scanDeadSymbol(root, sym)
		if err != nil {
			t.Fatalf("扫描 %s 失败: %v", sym, err)
		}
		if len(hits) > 0 {
			t.Errorf("墓碑符号 %q 复活（%d 处命中）：\n%s", sym, len(hits), strings.Join(hits, "\n"))
		}
	}
}
