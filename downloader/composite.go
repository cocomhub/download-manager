// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"errors"
	"fmt"
	"log/slog"
	"reflect"

	"github.com/cocomhub/download-manager/pkg/logutil"
)

// ErrCompositeEmpty 表示复合下载的文件列表为空，需要重新触发 Scrape。
var ErrCompositeEmpty = errors.New("composite: file list is empty, need re-scrape")

// convertMapAnyToStrMap 将 map[string]any 转换为 map[string]string，仅保留 string 类型的值。
func convertMapAnyToStrMap(m map[string]any) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}

// extractStrMapSlice 通过反射从 primitive.A 中提取 []map[string]string。
func extractStrMapSlice(v any) []map[string]string {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Slice {
		return nil
	}
	if fmt.Sprintf("%T", v) != "primitive.A" {
		return nil
	}

	result := make([]map[string]string, 0, val.Len())
	for i := 0; i < val.Len(); i++ {
		elem := val.Index(i).Interface()
		if fm, ok := elem.(map[string]any); ok {
			result = append(result, convertMapAnyToStrMap(fm))
		}
	}
	return result
}

// parseCompositeFiles 从 obj.Extra["files"] 解析文件列表。
// 统一处理 []map[string]string (memory存储)、[]any (JSON反序列化) 和
// primitive.A (MongoDB BSON数组) 三种来源。
func parseCompositeFiles(filesVal any) ([]map[string]string, error) {
	// Direct []map[string]string type (memory storage)
	if files, ok := filesVal.([]map[string]string); ok {
		if len(files) == 0 {
			return nil, ErrCompositeEmpty
		}
		return files, nil
	}

	// primitive.A (MongoDB BSON array) via reflection
	if fileList := extractStrMapSlice(filesVal); fileList != nil {
		if len(fileList) == 0 {
			return nil, ErrCompositeEmpty
		}
		return fileList, nil
	}

	// []any (JSON deserialized from memory storage)
	if files, ok := filesVal.([]any); ok {
		fileList := make([]map[string]string, 0, len(files))
		for _, f := range files {
			if fm, ok := f.(map[string]any); ok {
				fileList = append(fileList, convertMapAnyToStrMap(fm))
			}
		}
		if len(fileList) == 0 {
			return nil, ErrCompositeEmpty
		}
		return fileList, nil
	}

	slog.Error("Composite download with unknown files metadata type", logutil.LogKeyType, fmt.Sprintf("%T", filesVal))
	return nil, fmt.Errorf("composite download error: unknown 'files' metadata type")
}
