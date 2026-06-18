// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"testing"
)

// ================================================================
// 复合下载测试 — Extra["files"] 逻辑
// ================================================================

// TestComposite_SingleVideo 验证单个 video 子文件的进度追踪。
func TestComposite_SingleVideo(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/video.mp4", "video-content", "video/mp4")

	extra := map[string]any{
		"files": []map[string]string{
			{"url": b.URL() + "/video.mp4", "path": "composite/video.mp4", "type": "video"},
		},
	}
	obj := makeTestObject(b.URL()+"/video.mp4", "composite/video.mp4", nil, extra)

	cmp := NewComparator(t, b)
	cmp.Run("single-video", obj, nil)
}

// TestComposite_MultipleFiles 验证多个子文件全部下载成功。
func TestComposite_MultipleFiles(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/main.mp4", "main-content", "video/mp4")
	b.HandleFile("GET", "/cover.jpg", "cover-data", "image/jpeg")
	b.HandleFile("GET", "/sub.srt", "subtitle-data", "text/plain")

	extra := map[string]any{
		"files": []map[string]string{
			{"url": b.URL() + "/main.mp4", "path": "multi/main.mp4", "type": "video"},
			{"url": b.URL() + "/cover.jpg", "path": "multi/cover.jpg", "type": "cover"},
			{"url": b.URL() + "/sub.srt", "path": "multi/sub.srt", "type": "subtitle"},
		},
	}

	obj := makeTestObject(b.URL()+"/main.mp4", "multi/main.mp4", nil, extra)
	cmp := NewComparator(t, b)
	cmp.Run("multiple-files", obj, nil)
}

// TestComposite_EmptyFilesList 验证空 files 列表返回错误。
func TestComposite_EmptyFilesList(t *testing.T) {
	b := NewBeacon(t)
	extra := map[string]any{
		"files": []map[string]string{},
	}

	obj := makeTestObject(b.URL()+"/none.bin", "empty/out.bin", nil, extra)
	cmp := NewComparator(t, b)
	cmp.Run("empty-list", obj, nil, CheckAnyError())
}

// TestComposite_PartialFail 验证部分子文件失败时的行为。
func TestComposite_PartialFail(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/good.bin", "good content", "text/plain")
	b.HandleError("GET", "/bad.bin", 403)

	extra := map[string]any{
		"files": []map[string]string{
			{"url": b.URL() + "/good.bin", "path": "partial/good.bin", "type": "cover"},
			{"url": b.URL() + "/bad.bin", "path": "partial/bad.bin", "type": "video"},
		},
	}

	obj := makeTestObject(b.URL()+"/good.bin", "partial/main.bin", nil, extra)
	cmp := NewComparator(t, b)
	cmp.Run("partial-fail", obj, nil, CheckAnyError())
}

// TestComposite_MetadataPrefix 验证子文件的带前缀元数据。
func TestComposite_MetadataPrefix(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/cover.jpg", "cover-image-data", "image/jpeg")

	extra := map[string]any{
		"files": []map[string]string{
			{"url": b.URL() + "/cover.jpg", "path": "prefix/cover.jpg", "type": "cover"},
		},
	}

	obj := makeTestObject(b.URL()+"/cover.jpg", "prefix/main.mp4", nil, extra)
	cmp := NewComparator(t, b)
	cmp.Run("metadata-prefix", obj, nil)
}
