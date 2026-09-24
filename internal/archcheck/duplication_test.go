// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package archcheck

// duplication_test.go 是**重复实现门禁**：把「共享辅助必须走单一事实源」从文档约定
// 变成可执行断言（对齐 sproxy R5）。
//
// dm 的历史重复：pkg/m3u8d 与 pkg/download/m3u8d 曾是同源重复（P4-2 已收敛删除 pkg/m3u8d）。
// 本门禁防未来再次出现同签名重复定义。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sharedHelperGuards 是「不得在 pkg/ 下重新定义」的共享辅助清单。
var sharedHelperGuards = []struct {
	// signature 是本地定义的函数签名前缀（去空白后精确匹配）。
	signature string
	// replacement 是应当改用的共享实现（用于失败信息）。
	replacement string
	// why 说明「为什么必须单一事实源」。
	why string
}{
	{
		signature:   "func NewM3U8Downloader(",
		replacement: "pkg/download/m3u8d.NewM3U8DEngine",
		why: "m3u8 下载引擎必须单一事实源：pkg/m3u8d 旧实现已随 P4-2 收敛删除，" +
			"新引擎（支持 http.Client 注入 + 安全加固）是唯一实现。",
	},
}

// TestNoDuplicateSharedHelper 扫描根 module 的 .go 文件，断言共享辅助签名不被重新定义。
func TestNoDuplicateSharedHelper(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)

	var hits []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := strings.ReplaceAll(string(b), " ", "")
		for _, g := range sharedHelperGuards {
			if strings.Contains(text, g.signature) {
				rel, _ := filepath.Rel(root, path)
				hits = append(hits, rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if len(hits) > 0 {
		var msgs []string
		for _, g := range sharedHelperGuards {
			for _, h := range hits {
				msgs = append(msgs, g.signature+" 出现在 "+h+"；应改用 "+g.replacement+"（"+g.why+"）")
			}
		}
		t.Errorf("重复实现：\n%s", strings.Join(msgs, "\n"))
	}
}
