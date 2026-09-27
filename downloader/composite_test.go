// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestParseCompositeFiles_BSONA 验证 mongo 读回的 bson.A（元素 bson.D）能解析。
// 实测：mongo driver v2 对 map[string]any 的 Extra 解码时，嵌套数组元素是
// bson.D（ordered doc）而非 map[string]any → parseCompositeFiles 原实现报
// unknown 'files' metadata type。
func TestParseCompositeFiles_BSONA(t *testing.T) {
	t.Parallel()
	files := bson.A{
		bson.D{{Key: "type", Value: "image"}, {Key: "url", Value: "https://fourhoi.com/cover.jpg"}},
		bson.D{{Key: "type", Value: "video"}, {Key: "url", Value: "https://surrit.com/playlist.m3u8"}},
	}
	list, err := parseCompositeFiles(files)
	if err != nil {
		t.Fatalf("parseCompositeFiles(bson.A/bson.D) err: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	if list[0]["type"] != "image" || list[0]["url"] != "https://fourhoi.com/cover.jpg" {
		t.Errorf("image = %+v", list[0])
	}
	if list[1]["type"] != "video" {
		t.Errorf("video = %+v", list[1])
	}
}

// TestParseCompositeFiles_BSONM 回归：bson.A 元素是 bson.M 仍可解析。
func TestParseCompositeFiles_BSONM(t *testing.T) {
	t.Parallel()
	files := bson.A{
		bson.M{"type": "image", "url": "https://fourhoi.com/cover.jpg"},
	}
	list, err := parseCompositeFiles(files)
	if err != nil {
		t.Fatalf("parseCompositeFiles(bson.M) err: %v", err)
	}
	if len(list) != 1 || list[0]["url"] != "https://fourhoi.com/cover.jpg" {
		t.Errorf("list = %+v", list)
	}
}

// TestParseCompositeFiles_StrMap 回归：内存存储 []map[string]string 直接可用。
func TestParseCompositeFiles_StrMap(t *testing.T) {
	t.Parallel()
	files := []map[string]string{{"type": "video", "url": "https://x/playlist.m3u8"}}
	list, err := parseCompositeFiles(files)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(list) != 1 || list[0]["url"] != "https://x/playlist.m3u8" {
		t.Errorf("list = %+v", list)
	}
}
