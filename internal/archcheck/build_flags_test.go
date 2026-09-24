// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package archcheck

// build_flags_test.go 门禁：**发布产物（GoReleaser）与 `make build` 必须注入同一组
// 构建元信息**（-X 键集一致），并同样启用 -trimpath（对齐 sproxy R18）。
//
// 背景：`.goreleaser.yaml` 注入 `-X main.Version={{.Version}}` 与 `-X main.BuildAt={{.Date}}`，
// 而 `make build` 曾只注入 `-X main.Version=$(VERSION)`——版本号形式不一致（发布 v0.3.0 vs
// 本地 v0.3.0）会导致两者无法互相引用比对。

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var alignXRe = regexp.MustCompile(`-X\s+([A-Za-z0-9_./-]+)=(\S+)`)

func TestBuildFlagsAlignedBetweenMakeAndGoReleaser(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)

	mk := alignReadFile(t, filepath.Join(root, "Makefile"))
	mkKeys, mkVals := alignXFlags(mk)
	if len(mkKeys) == 0 {
		t.Fatal("未从 Makefile 解析到 -X 注入（判据失效，需更新本测试）")
	}
	if !strings.Contains(mk, "-trimpath") {
		t.Error("Makefile 缺少 -trimpath（go build 开关，须位于 -ldflags 之外）")
	}
	if got := mkVals["main.Version"]; got != "$(VERSION)" {
		t.Errorf("Makefile 的 main.Version 注入=%q，预期 $(VERSION)", got)
	}
	// 版本取值形式必须是「只认根 tag 的 git describe」：否则嵌套模块 tag 会抢走 describe。
	versionLine := alignFirstLinePrefixed(mk, "VERSION ")
	if !strings.Contains(versionLine, "--match 'v[0-9]*'") {
		t.Errorf("Makefile 的 VERSION 定义缺少 --match 'v[0-9]*'：%s", versionLine)
	}

	gr := alignReadFile(t, filepath.Join(root, ".goreleaser.yaml"))
	grKeys, _ := alignXFlags(gr)
	if len(grKeys) == 0 {
		t.Fatalf(".goreleaser.yaml 未解析到 -X 注入（判据失效）")
	}
	if got, want := strings.Join(alignSortedKeys(grKeys), ","), strings.Join(alignSortedKeys(mkKeys), ","); got != want {
		t.Errorf(".goreleaser.yaml 的 -X 键集与 Makefile 不一致：\n  goreleaser = [%s]\n  makefile   = [%s]",
			got, want)
	}
	if !strings.Contains(gr, "-trimpath") {
		t.Error(".goreleaser.yaml 缺少 flags: -trimpath（与 make build 的 -trimpath 对齐）")
	}
}

// alignReadFile 读取文件（失败即 fatal）。
func alignReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s: %v", path, err)
	}
	return string(b)
}

// alignXFlags 提取文本中全部 `-X key=value` 的 key → value 映射。
func alignXFlags(text string) (keys []string, vals map[string]string) {
	vals = map[string]string{}
	for _, m := range alignXRe.FindAllStringSubmatch(text, -1) {
		keys = append(keys, m[1])
		vals[m[1]] = m[2]
	}
	return keys, vals
}

// alignSortedKeys 返回排序后的 key 列表。
func alignSortedKeys(keys []string) []string {
	out := append([]string(nil), keys...)
	sort.Strings(out)
	return out
}

// alignFirstLinePrefixed 返回文本中第一个以 prefix 开头的行。
func alignFirstLinePrefixed(text, prefix string) string {
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}
