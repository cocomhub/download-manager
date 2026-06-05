// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"fmt"
	"log/slog"
)

// parseCompositeFiles 从 obj.Extra["files"] 解析文件列表。
// 统一处理 []map[string]string (memory存储) 和 []any (JSON反序列化) 两种来源。
func parseCompositeFiles(filesVal any) ([]map[string]string, error) {
	// Handle different types depending on source
	if files, ok := filesVal.([]map[string]string); ok {
		return files, nil
	}

	if files, ok := filesVal.([]any); ok {
		var fileList []map[string]string
		for _, f := range files {
			if fm, ok := f.(map[string]any); ok {
				m := make(map[string]string)
				for k, v := range fm {
					if s, ok := v.(string); ok {
						m[k] = s
					}
				}
				fileList = append(fileList, m)
			}
		}
		return fileList, nil
	}

	slog.Error("Composite download with unknown files metadata type", "type", fmt.Sprintf("%T", filesVal))
	return nil, fmt.Errorf("composite download error: unknown 'files' metadata type")
}