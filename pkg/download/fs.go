// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/cocomhub/download-manager/pkg/logutil"
)

// ResolvePath 将路径 p 解析为绝对路径并验证其位于 rootDir 之下。
// 如果 rootDir 为空字符串，则不进行路径限制（p 按原样返回）。
// followSymlinks 控制是否解析符号链接。
func ResolvePath(rootDir, p string, followSymlinks ...bool) (string, error) {
	fs := len(followSymlinks) == 0 || followSymlinks[0]
	return resolvePath(rootDir, p, fs)
}

// ResolvePathNoFollow 等价于 ResolvePath(rootDir, p, false)。
func ResolvePathNoFollow(rootDir, p string) (string, error) {
	return resolvePath(rootDir, p, false)
}

func resolvePath(rootDir, p string, followSymlinks bool) (string, error) {
	if rootDir == "" {
		return p, nil
	}
	if filepath.IsAbs(p) {
		if isWithinRoot(rootDir, p, followSymlinks) {
			return p, nil
		}
		return "", fmt.Errorf("path outside root: %s", p)
	}
	rp := cleanJoin(rootDir, p)
	if !isWithinRoot(rootDir, rp, followSymlinks) {
		return "", fmt.Errorf("path outside root: %s", p)
	}
	return rp, nil
}

// ResolvePathWithAllowList 在 ResolvePath 基础上增加白名单校验。
// 当 allowPaths 非空时，解析后的路径必须位于至少一个白名单目录下。
// 未配置白名单时行为与 ResolvePath 一致。
// followSymlinks 控制是否解析符号链接。
func ResolvePathWithAllowList(rootDir string, allowPaths []string, p string, followSymlinks ...bool) (string, error) {
	fs := len(followSymlinks) == 0 || followSymlinks[0]
	resolved, err := resolvePath(rootDir, p, fs)
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
		if fs {
			if evalAP, evalErr := filepath.EvalSymlinks(absAP); evalErr == nil {
				absAP = evalAP
			}
		}
		if strings.HasPrefix(resolved, absAP+string(filepath.Separator)) || resolved == absAP {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("path not in allowed list: %s", p)
}

// IsWithinRoot 检查 p 是否在 rootDir 的安全范围内。
// 默认解析符号链接，防止通过符号链接绕过路径检查。
// 当路径指向不存在的文件时，会逐级向上解析父目录以正确处理 macOS /tmp → /private/tmp 等系统符号链接。
// followSymlinks 控制是否解析符号链接。
func IsWithinRoot(rootDir, p string, followSymlinks ...bool) bool {
	fs := len(followSymlinks) == 0 || followSymlinks[0]
	return isWithinRoot(rootDir, p, fs)
}

// resolveSymlinksSafe 安全地解析路径的符号链接。
// 如果路径本身不存在，则逐级向上解析已存在的父目录组件，
// 以正确处理 macOS /tmp → /private/tmp 等系统符号链接。
func resolveSymlinksSafe(p string) (string, error) {
	resolved, err := filepath.EvalSymlinks(p)
	if err == nil {
		return resolved, nil
	}
	// 路径不存在，逐级向上查找已存在的祖先并解析其符号链接
	parent := filepath.Dir(p)
	if parent == p {
		// 已到根目录，返回原路径
		return p, nil
	}
	resolvedParent, err := resolveSymlinksSafe(parent)
	if err != nil {
		return p, nil
	}
	return filepath.Join(resolvedParent, filepath.Base(p)), nil
}

func isWithinRoot(rootDir, p string, followSymlinks bool) bool {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return false
	}
	absP, err := filepath.Abs(p)
	if err != nil {
		return false
	}

	if followSymlinks {
		// 解析符号链接，防止路径穿越绕过
		if resolvedRoot, err := filepath.EvalSymlinks(absRoot); err == nil {
			absRoot = resolvedRoot
		}
		if resolvedP, err := resolveSymlinksSafe(absP); err == nil {
			absP = resolvedP
		}
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
