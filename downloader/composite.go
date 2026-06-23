// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"errors"
	"fmt"
	"log/slog"
	"reflect"
)

// ErrCompositeEmpty 琛ㄧず澶嶅悎涓嬭浇鐨勬枃浠跺垪琛ㄤ负绌猴紝闇€瑕侀噸鏂拌Е鍙?Scrape銆?var ErrCompositeEmpty = errors.New("composite: file list is empty, need re-scrape")

// parseCompositeFiles 浠?obj.Extra["files"] 瑙ｆ瀽鏂囦欢鍒楄〃銆?// 缁熶竴澶勭悊 []map[string]string (memory瀛樺偍)銆乕]any (JSON鍙嶅簭鍒楀寲) 鍜?// primitive.A (MongoDB BSON鏁扮粍) 涓夌鏉ユ簮銆?func parseCompositeFiles(filesVal any) ([]map[string]string, error) {
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
			if len(fileList) == 0 {
				return nil, ErrCompositeEmpty
			}
			return fileList, nil
		}
	}

	// Handle direct []map[string]string type
	if files, ok := filesVal.([]map[string]string); ok {
		if len(files) == 0 {
			return nil, ErrCompositeEmpty
		}
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
		if len(fileList) == 0 {
			return nil, ErrCompositeEmpty
		}
		return fileList, nil
	}

	slog.Error("Composite download with unknown files metadata type", "type", fmt.Sprintf("%T", filesVal))
	return nil, fmt.Errorf("composite download error: unknown 'files' metadata type")
}
