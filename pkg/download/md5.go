// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
	"strings"
)

// ComputeFileMD5 璁＄畻鏂囦欢鐨?MD5 鏍￠獙鍊硷紝杩斿洖 Base64 鍜屽崄鍏繘鍒朵袱绉嶆牸寮忋€?func ComputeFileMD5(filePath string) (base64MD5, hexMD5 string, err error) {
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

// TryGetMd5 灏濊瘯浠庡搷搴斿ご涓彁鍙?MD5 鍊笺€傛寜浠ヤ笅椤哄簭灏濊瘯锛?//  1. X-Amz-Meta-Md5chksum锛?4 瀛楃 Base64锛?//  2. Etag锛堟牸寮?"32hex"锛岄暱搴︿负 34锛屽幓闄ゅ紩鍙凤級
//  3. Content-MD5锛?2 瀛楃 hex锛?//
// 鎵€鏈夋潯浠朵笉婊¤冻鏃惰繑鍥炵┖瀛楃涓层€?func TryGetMd5(headers map[string]string) string {
	if headers == nil {
		return ""
	}

	if x := headers["X-Amz-Meta-Md5chksum"]; len(x) == 24 {
		return x
	}
	if etag := headers["Etag"]; len(etag) == 34 && etag[0] == '"' && etag[33] == '"' {
		return etag[1:33]
	}
	// 寮?ETag 鏀寔锛氬鐞?W/"32hex" 鏍煎紡锛?6 瀛楃锛?	if etag := headers["Etag"]; len(etag) == 36 && (strings.HasPrefix(etag, `W/"`) || strings.HasPrefix(etag, `w/"`)) && etag[35] == '"' {
		return etag[3:35]
	}
	if x := headers["Content-MD5"]; len(x) == 32 {
		return x
	}
	// Go 鏍囧噯搴?canonical 灏?Content-MD5 杞负 Content-Md5
	if x := headers["Content-Md5"]; len(x) == 32 {
		return x
	}
	return ""
}
