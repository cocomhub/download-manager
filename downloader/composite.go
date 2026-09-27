// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"errors"
	"fmt"
	"log/slog"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"

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

// extractStrMapSlice 通过反射从 BSON 数组（bson.A / primitive.A / 任意 slice）中
// 提取 []map[string]string。v2 的 bson.A 是独立类型（type A []any），v1 的
// primitive.A 是其旧名——两者 %T 不同，统一按 slice 反射处理，不锁死类型名。
func extractStrMapSlice(v any) []map[string]string {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Slice {
		return nil
	}

	result := make([]map[string]string, 0, val.Len())
	for i := 0; i < val.Len(); i++ {
		elem := val.Index(i).Interface()
		switch fm := elem.(type) {
		case map[string]any:
			result = append(result, convertMapAnyToStrMap(fm))
		case bson.M:
			// bson.M = type M map[string]any（命名类型，断言 map[string]any 失败）。
			result = append(result, convertMapAnyToStrMap(map[string]any(fm)))
		case bson.D:
			// mongo driver v2 对 map[string]any 的 Extra 解码时，嵌套数组元素默认是
			// bson.D（ordered doc）而非 bson.M——必须支持，否则 composite 下载报
			// unknown 'files' metadata type。
			result = append(result, convertBSONDToStrMap(fm))
		}
	}
	return result
}

// convertBSONDToStrMap 把 bson.D（ordered doc）转为 map[string]string（仅 string 值）。
func convertBSONDToStrMap(d bson.D) map[string]string {
	m := make(map[string]string, len(d))
	for _, kv := range d {
		if s, ok := kv.Value.(string); ok {
			m[kv.Key] = s
		}
	}
	return m
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
			switch fm := f.(type) {
			case map[string]any:
				fileList = append(fileList, convertMapAnyToStrMap(fm))
			case bson.M:
				fileList = append(fileList, convertMapAnyToStrMap(map[string]any(fm)))
			case bson.D:
				fileList = append(fileList, convertBSONDToStrMap(fm))
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
