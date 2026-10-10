// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"reflect"
	"testing"
)

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
		if !f.IsExported() {
			continue
		}
		if sv.Field(i).IsZero() {
			t.Errorf("Snapshot 漏拷字段 %s（源值为 %v）", f.Name, dv.Field(i).Interface())
		}
	}
}

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
