// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"encoding/json"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/cocomhub/download-manager/model"
)

// TestExtraMap_NormalizesBSON 钉住 mongo 读回形态（bson.D/bson.M）可读：
// meta_extra 透传曾因 .(map[string]any) 失败而静默丢失（2026-10-28 修复）。
func TestExtraMap_NormalizesBSON(t *testing.T) {
	t.Parallel()
	obj := &model.DownloadObject{
		Extra: map[string]any{
			"meta_extra": map[string]any{"code": "EXAMPLE-001", "genres": []string{"类型"}},
			"files": []any{
				map[string]any{"url": "https://example.com/s/x", "path": "a.mp4", "target": "a.mp4"},
			},
		},
	}
	// mongo 存取往返
	b, err := bson.Marshal(obj)
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}
	var back model.DownloadObject
	if uerr := bson.Unmarshal(b, &back); uerr != nil {
		t.Fatalf("bson.Unmarshal: %v", uerr)
	}
	me := extraMap(&back, "meta_extra")
	if me["code"] != "EXAMPLE-001" {
		t.Fatalf("meta_extra 归一失败: %v", me)
	}
	if g, _ := me["genres"].([]any); len(g) != 1 {
		t.Fatalf("数组未归一为 []any: %#v", me["genres"])
	}
	// 归一结果必须可 JSON 序列化（bson.D 直接 Marshal 会变成 [{Key,Value}] 数组）
	if _, jerr := json.Marshal(me); jerr != nil {
		t.Fatalf("归一结果不可 JSON 序列化: %v", jerr)
	}

	targets := cloudTargets(&back)
	if len(targets) != 1 || targets[0].Target != "a.mp4" {
		t.Fatalf("cloudTargets 解析失败: %+v", targets)
	}
}

// TestCloudTargets_BSONNestedMeta 验证 BSON 读回后多目标 per-file meta 仍可读取
// （分卷/字幕的 share_file_name 依赖它）。
func TestCloudTargets_BSONNestedMeta(t *testing.T) {
	t.Parallel()
	obj := &model.DownloadObject{Extra: map[string]any{
		"files": []map[string]any{
			{"url": "https://example.com/s/x", "path": "p1.mp4", "target": "p1.mp4",
				"meta": map[string]any{"share_file_name": "p1.mp4", "x_role": "video"}},
			{"url": "https://example.com/s/x", "path": "p2.srt", "target": "p2.srt",
				"meta": map[string]any{"share_file_name": "p2.srt", "x_role": "subtitle"}},
		},
	}}
	b, _ := bson.Marshal(obj)
	var back model.DownloadObject
	if err := bson.Unmarshal(b, &back); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}
	targets := cloudTargets(&back)
	if len(targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(targets))
	}
	if targets[1].Meta["share_file_name"] != "p2.srt" || targets[1].Meta["x_role"] != "subtitle" {
		t.Fatalf("per-file meta 丢失: %+v", targets[1].Meta)
	}
}
