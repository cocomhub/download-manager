// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"strings"
)

// ComputeFileMD5 计算文件的 MD5 校验值，返回 Base64 和十六进制两种格式。
func ComputeFileMD5(filePath string) (base64MD5, hexMD5 string, err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	hasher := md5.New()
	buf := make([]byte, 64*1024)
	if _, err := io.CopyBuffer(hasher, file, buf); err != nil {
		return "", "", err
	}

	hashBytes := hasher.Sum(nil)
	return base64.StdEncoding.EncodeToString(hashBytes), hex.EncodeToString(hashBytes), nil
}

const (
	// etagQuotedLen 是标准双引号包裹的 ETag 长度：2 个引号 + 32 个 hex 字符 = 34
	etagQuotedLen = 34
	// weakETagQuotedLen 是弱 ETag 长度：W/" 前缀 + 32 个 hex 字符 + 引号 = 36
	weakETagQuotedLen = 36
)

// TryGetMd5 尝试从响应头中提取 MD5 值。按以下顺序尝试：
//  1. X-Amz-Meta-Md5chksum（24 字符 Base64）
//  2. Etag（格式 "32hex"，长度为 34，去除引号）
//  3. 弱 ETag（格式 W/"32hex"，长度为 36，去除前缀）
//  4. 其他长度的 ETag，只要以引号包裹且内部为 32 字符 hex 则提取
//  5. Content-MD5（32 字符 hex 或 24 字符 Base64 标准格式）
//
// 所有条件不满足时返回空字符串。
func TryGetMd5(headers map[string]string) string {
	if headers == nil {
		return ""
	}

	if x := headers["X-Amz-Meta-Md5chksum"]; len(x) == 24 {
		return x
	}
	if etag := headers["Etag"]; len(etag) == etagQuotedLen && etag[0] == '"' && etag[etagQuotedLen-1] == '"' {
		inner := etag[1:33]
		if _, err := hex.DecodeString(inner); err == nil {
			return inner
		}
		slog.Debug("Strong ETag content is not valid hex, skipping MD5 extraction", "etag", etag)
	}
	// 弱 ETag 支持：处理 W/"32hex" 格式（36 字符）
	if etag := headers["Etag"]; len(etag) == weakETagQuotedLen && (strings.HasPrefix(etag, `W/"`) || strings.HasPrefix(etag, `w/"`)) && etag[weakETagQuotedLen-1] == '"' {
		inner := etag[3:35]
		if _, err := hex.DecodeString(inner); err == nil {
			return inner
		}
		slog.Debug("Weak ETag content is not valid hex, skipping MD5 extraction", "etag", etag)
	}
	// 非标准长度 ETag：放宽长度检查，但验证 hex 格式
	if etag := headers["Etag"]; len(etag) > 2 && etag[0] == '"' && etag[len(etag)-1] == '"' {
		inner := etag[1 : len(etag)-1]
		if len(inner) == 32 {
			if _, err := hex.DecodeString(inner); err == nil {
				return inner
			}
		}
		slog.Debug("Non-standard ETag length or format, skipping MD5 extraction", "etag", etag, "len", len(inner))
	}
	// Content-MD5 — 不区分大小写匹配
	for k, v := range headers {
		if !strings.EqualFold(k, "Content-MD5") {
			continue
		}
		// 24 字符 Base64 标准格式
		if len(v) == 24 {
			decoded, err := base64.StdEncoding.DecodeString(v)
			if err == nil {
				return hex.EncodeToString(decoded)
			}
			return v
		}
		// 32 字符 hex 格式（非标准，兼容处理）
		if len(v) == 32 {
			if _, err := hex.DecodeString(v); err == nil {
				return v
			}
		}
	}
	return ""
}
