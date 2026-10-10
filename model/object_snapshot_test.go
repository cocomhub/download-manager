// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestSnapshot_BSONKeepsFalseCloudDownload 防止 bson omitempty 使「关闭开关」无法落库：
// MongoStorage.Update 以 Snapshot 作为 $set 唯一来源，字段被省略时旧值（true）会残留。
func TestSnapshot_BSONKeepsFalseCloudDownload(t *testing.T) {
	t.Parallel()
	snap := (&DownloadObject{URL: "u"}).Snapshot()
	raw, err := bson.Marshal(snap)
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}
	var m bson.M
	if err := bson.Unmarshal(raw, &m); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}
	if _, ok := m["cloud_download"]; !ok {
		t.Fatalf("bson 缺 cloud_download（omitempty）→ mongo $set 无法把开关关回去；keys=%v", bsonKeys(m))
	}
}

func bsonKeys(m bson.M) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestSnapshot_CoversAllExportedFields 防止 Snapshot() 漏拷字段。
// 背景：曾漏拷 CloudDownload → MongoStorage.Update 用 Snapshot 作为 $set 唯一来源，
// 该字段（带 omitempty）永不落库 ⇒ 「云端下载」开关在 mongo 后端静默失效。
func TestSnapshot_CoversAllExportedFields(t *testing.T) {
	t.Parallel()
	src := &DownloadObject{
		TaskID:        "task-1",
		URL:           "http://example.com/x.mp4",
		ID:            7,
		SavePath:      "/tmp/x.mp4",
		Status:        StatusPending,
		Progress:      42,
		Version:       3,
		CloudDownload: true,
		Metadata:      map[string]string{"k": "v"},
		Extra:         map[string]any{"a": 1},
	}

	snap := src.Snapshot()
	if snap == nil {
		t.Fatal("Snapshot returned nil")
	}

	sv := reflect.ValueOf(snap).Elem()
	dv := reflect.ValueOf(src).Elem()
	typ := sv.Type()
	for i := range typ.NumField() {
		f := typ.Field(i)
		if !f.IsExported() || snapshotExcludedFields[f.Name] {
			continue
		}
		if sv.Field(i).IsZero() {
			t.Errorf("Snapshot 漏拷字段 %s（源值为 %v）", f.Name, dv.Field(i).Interface())
		}
	}
	// 排除清单不得残留已不存在的字段名（否则会静默掩盖新字段漏拷）
	for name := range snapshotExcludedFields {
		if _, ok := typ.FieldByName(name); !ok {
			t.Errorf("snapshotExcludedFields 含不存在的字段 %s", name)
		}
	}
}

// snapshotExcludedFields 列出「有意不写入 Snapshot」的导出字段（当前为空）。
// 新增此类字段时须显式登记，避免测试以「合法零值」误报或静默放过漏拷。
var snapshotExcludedFields = map[string]bool{}

// TestSnapshot_IsolatesMaps 验证 Snapshot 深拷贝 Metadata/Extra（并发安全前提）。
func TestSnapshot_IsolatesMaps(t *testing.T) {
	t.Parallel()
	src := &DownloadObject{
		Metadata: map[string]string{"k": "v"},
		Extra:    map[string]any{"a": 1},
	}
	snap := src.Snapshot()
	snap.Metadata["k"] = "changed"
	snap.Extra["a"] = 2
	if src.Metadata["k"] != "v" {
		t.Error("Snapshot.Metadata 未深拷贝")
	}
	if src.Extra["a"] != 1 {
		t.Error("Snapshot.Extra 未深拷贝")
	}
}
