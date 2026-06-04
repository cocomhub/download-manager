// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
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

// TryGetMd5 尝试从响应头中提取 MD5 值。按以下顺序尝试：
//  1. X-Amz-Meta-Md5chksum（24 字符 Base64）
//  2. Etag（格式 "32hex"，长度为 34，去除引号）
//  3. Content-MD5（32 字符 hex）
//
// 所有条件不满足时返回空字符串。
func TryGetMd5(headers map[string]string) string {
	if headers == nil {
		return ""
	}

	if x := headers["X-Amz-Meta-Md5chksum"]; len(x) == 24 {
		return x
	}
	if etag := headers["Etag"]; len(etag) == 34 && etag[0] == '"' && etag[33] == '"' {
		return etag[1:33]
	}
	if x := headers["Content-MD5"]; len(x) == 32 {
		return x
	}
	return ""
}