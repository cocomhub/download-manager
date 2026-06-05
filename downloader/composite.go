// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"fmt"
	"log/slog"
	"reflect"
)

// parseCompositeFiles 从 obj.Extra["files"] 解析文件列表。
// 统一处理 []map[string]string (memory存储)、[]any (JSON反序列化) 和
// primitive.A (MongoDB BSON数组) 三种来源。
func parseCompositeFiles(filesVal any) ([]map[string]string, error) {
	// Handle primitive.A (MongoDB BSON array) - convert to []any first
	val := reflect.ValueOf(filesVal)
	if val.Kind() == reflect.Slice {
		// Check if it's primitive.A by trying to convert to []any
		typeName := fmt.Sprintf("%T", filesVal)
		if typeName == "primitive.A" {
			var fileList []map[string]string
			for i := 0; i < val.Len(); i++ {
				elem := val.Index(i).Interface()
				if fm, ok := elem.(map[string]any); ok {
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
	}

	// Handle direct []map[string]string type
	if files, ok := filesVal.([]map[string]string); ok {
		return files, nil
	}

	// Handle []any (JSON deserialized from memory storage)
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
