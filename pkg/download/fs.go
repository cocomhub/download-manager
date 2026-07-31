// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/cocomhub/download-manager/pkg/logutil"
)

// ResolvePath 将路径 p 解析为绝对路径并验证其位于 rootDir 之下。
// 如果 rootDir 为空字符串，则不进行路径限制（p 按原样返回）。
func ResolvePath(rootDir, p string) (string, error) {
	if rootDir == "" {
		return p, nil
	}
	if filepath.IsAbs(p) {
		if isWithinRoot(rootDir, p) {
			return p, nil
		}
		return "", fmt.Errorf("path outside root: %s", p)
	}
	rp := cleanJoin(rootDir, p)
	if !isWithinRoot(rootDir, rp) {
		return "", fmt.Errorf("path outside root: %s", p)
	}
	return rp, nil
}

// ResolvePathWithAllowList 在 ResolvePath 基础上增加白名单校验。
// 当 allowPaths 非空时，解析后的路径必须位于至少一个白名单目录下。
// 未配置白名单时行为与 ResolvePath 一致。
func ResolvePathWithAllowList(rootDir string, allowPaths []string, p string) (string, error) {
	resolved, err := ResolvePath(rootDir, p)
	if err != nil {
		return "", err
	}
	if len(allowPaths) == 0 {
		return resolved, nil
	}
	for _, ap := range allowPaths {
		absAP, aErr := filepath.Abs(ap)
		if aErr != nil {
			slog.Warn("Failed to resolve allow path", "path", ap, logutil.LogKeyError, aErr)
			continue
		}
		if strings.HasPrefix(resolved, absAP+string(filepath.Separator)) || resolved == absAP {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("path not in allowed list: %s", p)
}

// isWithinRoot 检查 p 是否在 rootDir 的安全范围内。
// 对 p 和 rootDir 都解析符号链接，防止通过符号链接绕过路径检查。
func isWithinRoot(rootDir, p string) bool {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return false
	}
	absP, err := filepath.Abs(p)
	if err != nil {
		return false
	}

	// 解析符号链接，防止路径穿越绕过
	if resolvedRoot, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolvedRoot
	}
	if resolvedP, err := filepath.EvalSymlinks(absP); err == nil {
		absP = resolvedP
	}

	if absRoot == absP {
		return true
	}
	if !strings.HasSuffix(absRoot, string(filepath.Separator)) {
		absRoot += string(filepath.Separator)
	}
	return strings.HasPrefix(absP, absRoot)
}

// cleanJoin 将 rootDir 与任意元素拼接并用 filepath.Clean 规范化。
func cleanJoin(rootDir string, elems ...string) string {
	all := append([]string{rootDir}, elems...)
	return filepath.Clean(filepath.Join(all...))
}

// EnsureDir 确保文件路径的父目录存在（如 MkdirAll）。
func EnsureDir(path string) error {
	dir := filepath.Dir(path)
	if dir != "" {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}
