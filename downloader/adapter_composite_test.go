// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"testing"
)

// ================================================================
// 澶嶅悎涓嬭浇娴嬭瘯 鈥?Extra["files"] 閫昏緫
// ================================================================

// TestComposite_SingleVideo 楠岃瘉鍗曚釜 video 瀛愭枃浠剁殑杩涘害杩借釜銆?func TestComposite_SingleVideo(t *testing.T) {
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

// TestComposite_MultipleFiles 楠岃瘉澶氫釜瀛愭枃浠跺叏閮ㄤ笅杞芥垚鍔熴€?func TestComposite_MultipleFiles(t *testing.T) {
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

// TestComposite_EmptyFilesList 楠岃瘉绌?files 鍒楄〃杩斿洖閿欒銆?func TestComposite_EmptyFilesList(t *testing.T) {
	b := NewBeacon(t)
	extra := map[string]any{
		"files": []map[string]string{},
	}

	obj := makeTestObject(b.URL()+"/none.bin", "empty/out.bin", nil, extra)
	cmp := NewComparator(t, b)
	cmp.Run("empty-list", obj, nil, CheckAnyError())
}

// TestComposite_PartialFail 楠岃瘉閮ㄥ垎瀛愭枃浠跺け璐ユ椂鐨勮涓恒€?func TestComposite_PartialFail(t *testing.T) {
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

// TestComposite_MetadataPrefix 楠岃瘉瀛愭枃浠剁殑甯﹀墠缂€鍏冩暟鎹€?func TestComposite_MetadataPrefix(t *testing.T) {
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
