// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// extra_normalize.go 把 DownloadObject.Extra 的 **mongo/BSON 读回形态**归一为普通 Go 值。
//
// 为什么需要（2026-10-28，与 sdserver 同源缺陷）：对象经 Storage().Search() 从 mongo
// 读回后，Extra 里的内嵌文档是 `bson.D`、数组是 `bson.A`（命名类型）——
// `.(map[string]any)` / `.([]any)` 断言恒失败。此前表现为：`meta_extra` 透传失效
// （转存产物缺业务元数据）、多目标 `files` 解析为空（分卷/字幕不下载）。
//
// 归一结果：map[string]any / []any / 标量（JSON 可直接序列化）。

package downloader

import "reflect"

// normalizeExtra 递归归一 Extra 值（bson.D/bson.M/bson.A → map[string]any/[]any）。
func normalizeExtra(v any) any {
	switch t := v.(type) {
	case nil, string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, []byte:
		return v
	case map[string]any:
		for k, vv := range t {
			t[k] = normalizeExtra(vv)
		}
		return t
	case map[string]string:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			out[k] = vv
		}
		return out
	case []any:
		for i := range t {
			t[i] = normalizeExtra(t[i])
		}
		return t
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		// bson.D = []bson.E{Key string; Value any}（元素是带 Key/Value 字段的结构体）。
		if rv.Len() > 0 && rv.Type().Elem().Kind() == reflect.Struct &&
			rv.Type().Elem().NumField() >= 2 &&
			rv.Type().Elem().Field(0).Name == "Key" {
			m := make(map[string]any, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				e := rv.Index(i)
				m[e.Field(0).String()] = normalizeExtra(e.Field(1).Interface())
			}
			return m
		}
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = normalizeExtra(rv.Index(i).Interface())
		}
		return out
	case reflect.Map:
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out[iter.Key().String()] = normalizeExtra(iter.Value().Interface())
		}
		return out
	}
	return v
}
