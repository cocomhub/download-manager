// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolvePath 将给定的路径 p 相对于 rootDir 进行解析。
//   - 如果 rootDir 为空，直接返回 p。
//   - 如果 p 是绝对路径且在 rootDir 内，原样返回。
//   - 如果 p 是相对路径，与 rootDir 拼接后返回。
//   - 任何试图逃逸 rootDir 的行为均返回错误。
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
	rp, err := cleanJoin(rootDir, p)
	if err != nil {
		return "", err
	}
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
			continue
		}
		if strings.HasPrefix(resolved, absAP+string(filepath.Separator)) || resolved == absAP {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("path not in allowed list: %s", p)
}

// isWithinRoot 检查 p 是否在 rootDir 的安全范围内。
func isWithinRoot(rootDir, p string) bool {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return false
	}
	absP, err := filepath.Abs(p)
	if err != nil {
		return false
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
func cleanJoin(rootDir string, elems ...string) (string, error) {
	all := append([]string{rootDir}, elems...)
	return filepath.Clean(filepath.Join(all...)), nil
}

// EnsureDir 确保文件路径的父目录存在（如 MkdirAll）。
func EnsureDir(path string) error {
	dir := filepath.Dir(path)
	if dir != "" {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}
