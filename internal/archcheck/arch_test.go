// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package archcheck

// arch_test.go 是 dm 的分层方向性门禁（对齐 sproxy R1）：
// 已登记包不得导入层级更高的已登记包。
//
// 设计要点（对齐 sproxy）：
//   - 用 `go list` 子进程解析导入图，避免自己实现模块解析；
//   - 用 runtime.Caller 定位仓库根（go test 以包目录为 cwd，固定相对路径会失效）；
//   - 防空转：scopeAnchor 必须出现在图里（挡「图收缩成只剩本包的小子图」）。

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

const modulePrefix = "github.com/cocomhub/download-manager/"

// scopeAnchor 是导入图的锚点包：它必然存在、且必然属于根 module 的图。
// 用于挡「图收缩成只含 Managed 包的小子图」。
const scopeAnchor = modulePrefix + "manager"

// importGraph 调 `go list` 解析工作区导入图（直接导入，不含测试导入）。
func importGraph(t *testing.T) map[string][]string {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}|{{join .Imports \" \"}}", "./...")
	cmd.Dir = moduleRoot(t)
	// 用 Output() 只取 stdout：stderr 的 `go: downloading ...` 进度消息在冷缓存时
	// 会混入 CombinedOutput，被误当作包名导致「未登记 Levels」误报。
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list 失败（cwd=%s，需要 go 在 PATH 中）: %v\n--- go list stderr ---\n%s",
			cmd.Dir, err, stderrBuf.String())
	}
	graph := map[string][]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		pkg := parts[0]
		if len(parts) == 1 || strings.TrimSpace(parts[1]) == "" {
			graph[pkg] = nil
			continue
		}
		graph[pkg] = strings.Fields(parts[1])
	}

	// 防空转：作用域一旦收缩（cmd.Dir 失效），图里就只剩本包，断言会静默变绿。
	for pkg := range Levels {
		if _, ok := graph[pkg]; !ok {
			t.Fatalf("导入图缺少已登记包 %s（go list 作用域错误？图中共 %d 个包）", pkg, len(graph))
		}
	}
	if _, ok := graph[scopeAnchor]; !ok {
		t.Fatalf("导入图缺少锚点包 %s（可能：go list 作用域收缩；或锚点包已改名/迁走，请同步 scopeAnchor）（图中共 %d 个包）",
			scopeAnchor, len(graph))
	}
	return graph
}

// TestAllTopLevelPackagesRegistered 断言：根 module 的所有顶层包（除 cmd/ 与 testutil/）必须登记 Levels。
// 防「新包漏登」——漏登的包对 R1（分层方向）隐形。
func TestAllTopLevelPackagesRegistered(t *testing.T) {
	graph := importGraph(t)
	prefix := "github.com/cocomhub/download-manager/"
	for pkg := range graph {
		rel := strings.TrimPrefix(pkg, prefix)
		if rel == "" || pkg == prefix[:len(prefix)-1] || strings.HasPrefix(rel, "cmd/") ||
			strings.HasPrefix(rel, "testutil/") || rel == "internal/archcheck" {
			continue
		}
		if _, ok := Levels[pkg]; !ok {
			t.Errorf("包 %s 未登记 Levels（新增包必须登记层级，否则分层门禁对它失效）", pkg)
		}
	}
}

// TestLayeringDirection 断言：已登记包不得导入层级更高的已登记包。
func TestLayeringDirection(t *testing.T) {
	graph := importGraph(t)
	var violations []string
	for pkg, imports := range graph {
		own, ok := Levels[pkg]
		if !ok {
			continue
		}
		for _, imp := range imports {
			other, ok := Levels[imp]
			if !ok {
				continue
			}
			if other > own {
				violations = append(violations, pkg+"(L"+itoa(own)+") -> "+imp+"(L"+itoa(other)+")")
			}
		}
	}
	if len(violations) > 0 {
		t.Errorf("分层违规：低层不得导入高层：\n%s", strings.Join(violations, "\n"))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
