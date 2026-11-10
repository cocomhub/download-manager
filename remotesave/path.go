// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	maxRemoteSegmentBytes = 200
	maxRemotePathBytes    = 1024
)

// LocalRel 返回 abs 相对 base 的 slash 路径；拒绝越界。
func LocalRel(base, abs string) (string, error) {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	absClean, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, absClean)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == "" || strings.HasPrefix(rel, "../") || rel == ".." || path.IsAbs(rel) {
		return "", fmt.Errorf("路径不在任务目录内: %s", abs)
	}
	return rel, nil
}

// sanitizeRemoteRel 逐段清洗并拼接卷内相对路径。
func sanitizeRemoteRel(rel string) (string, error) {
	rel = strings.TrimSpace(strings.ReplaceAll(rel, `\`, "/"))
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || strings.ContainsRune(rel, 0) {
		return "", fmt.Errorf("非法相对路径: %q", rel)
	}
	segs := strings.Split(rel, "/")
	out := make([]string, 0, len(segs))
	for i, seg := range segs {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("非法相对路径段: %q", rel)
		}
		out = append(out, sanitizeRemoteSegment(seg, i == 0))
	}
	full := strings.Join(out, "/")
	if len(full) > maxRemotePathBytes {
		return "", fmt.Errorf("远端路径过长（%d > %d）: %s", len(full), maxRemotePathBytes, full)
	}
	return full, nil
}

// sanitizeRemoteSegment 使单段满足 sproxy storage.ValidSegmentName 的约束：
// 拒绝 / \ < > : " | ? *、控制字符、.__ 前缀（任意深度）、首段 __ 前缀；
// 去尾部点/空格；上限 255 字节（这里保守到 200）；Windows 保留设备名加后缀。
func sanitizeRemoteSegment(seg string, first bool) string {
	var b strings.Builder
	b.Grow(len(seg))
	for _, r := range seg {
		switch {
		case r < 0x20 || strings.ContainsRune(`/\<>:"|?*`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimRight(b.String(), ". ")
	if out == "" {
		out = "_"
	}
	if strings.HasPrefix(out, ".__") || (first && strings.HasPrefix(out, "__")) {
		out = "_" + out
	}
	if isWindowsReserved(out) {
		out += "_"
	}
	if len(out) > maxRemoteSegmentBytes {
		sum := sha256.Sum256([]byte(out))
		suffix := "~" + hex.EncodeToString(sum[:4])
		limit := maxRemoteSegmentBytes - len(suffix)
		for limit > 0 && !utf8.RuneStart(out[limit]) {
			limit--
		}
		out = out[:limit] + suffix
	}
	return out
}

func isWindowsReserved(name string) bool {
	base := name
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	}
	return false
}

// IsExcluded 按任务配置的 glob 排除（相对路径，支持 * ? 与 ** 前缀目录）。
func IsExcluded(rel string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.TrimSpace(strings.TrimPrefix(p, "/"))
		if p == "" {
			continue
		}
		if before, ok := strings.CutSuffix(p, "/**"); ok {
			prefix := before
			if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
				return true
			}
		}
		if ok, _ := path.Match(p, rel); ok {
			return true
		}
	}
	return false
}
