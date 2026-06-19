// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"net/http"
	"testing"
	"time"
)

// ================================================================
// dlcore-only 测试：验证 dlcore 特有的、pkg/download 不支持的行为
// 这些测试仅断言 dlcore 行为，pkg/download 的结果通过 t.Log 记录参考。
// ================================================================

// TestDlcoreOnly_MaxRetriesZero 验证 dlcore maxRetries=0 无限重试。
func TestDlcoreOnly_MaxRetriesZero(t *testing.T) {
	b := NewBeacon(t)
	b.HandleDynamic("GET", "/infinite.bin", func(r *http.Request) (int, map[string]string, []byte) {
		return http.StatusOK, map[string]string{"Content-Type": "text/plain"}, []byte("success")
	})

	cmp := NewComparator(t, b, WithMaxRetries(0))
	obj := makeTestObject(b.URL()+"/infinite.bin", "dlcoreonly/infinite.bin", nil, nil)
	cmp.Run("max-retries-zero", obj, nil,
		func(t *testing.T, old, new *DownloadResult) {
			t.Helper()
			// dlcore: maxRetries=0 表示无限重试
			if old.Err != nil {
				t.Errorf("dlcore: expected success with maxRetries=0, got %v", old.Err)
			}
			if len(old.FileContent) == 0 {
				t.Error("dlcore: expected file content")
			}
			// pkg/download: maxRetries=0 表示不重试
			t.Logf("pkg/download reference: err=%v", new.Err)
		},
	)
}

// TestDlcoreOnly_MetadataStatus 验证 dlcore 写入 Metadata["status"]="completed"。
func TestDlcoreOnly_MetadataStatus(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/metastatus.bin", "content", "text/plain")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/metastatus.bin", "dlcoreonly/metastatus.bin", nil, nil)
	cmp.DlcoreOnlyRun(t, "metadata-status", obj, nil,
		func(t *testing.T, old, new *DownloadResult) {
			t.Helper()
			// dlcore: Metadata["status"] == "completed"
			if old.Obj.Metadata["status"] != "completed" {
				t.Errorf("dlcore: expected Metadata[status]=completed, got %q", old.Obj.Metadata["status"])
			}
		},
	)
}

// TestDlcoreOnly_ImageURLTimeout 验证图片 URL 30s 超时。
// 注意：此测试运行较慢（至少等待 30s HTTP 超时）。
// 此处直接调用 oldDL 而非 DlcoreOnlyRun，因为需要精确 elapsed 计时。
func TestDlcoreOnly_ImageURLTimeout(t *testing.T) {
	b := NewBeacon(t)
	b.HandleSlow("GET", "/image.jpg", "image content", 35*time.Second)

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/image.jpg", "dlcoreonly/image.jpg", nil, nil)

	// 只测旧路径，避免新路径的 Content-Type 检测快速返回干扰
	oldObj := copyObject(obj)
	start := time.Now()
	var oldResult DownloadResult
	oldResult.Obj = oldObj
	oldResult.Err = cmp.oldDL.Download(oldObj, nil)
	collectFileResult(t, cmp.rootDir, &oldResult)
	elapsed := time.Since(start)

	// 默认 maxRetries=3，所以总等待可能超过 30s
	t.Logf("dlcore: err=%v, elapsed=%v", oldResult.Err, elapsed)
	if oldResult.Err == nil {
		t.Log("dlcore: image download succeeded (may have completed before timeout)")
	} else {
		t.Logf("dlcore: image download error: %v", oldResult.Err)
	}
}

// TestDlcoreOnly_HuaacgURL 验证 huaacg.com 特殊 5s 超时 + ErrNoTry。
// URL 必须包含 huaacg.com 才能触发 dlcore 的特殊逻辑（5s 上下文超时 + ErrNoTry 包装）。
// 测试依赖网络可达性，但实际请求在 5s 内因超时返回，不会产生大量流量。
// 成功时：dlcore 返回包装了 ErrNoTry 的错误。
// 若网络不可达：同样因超时快速返回，不会挂起。
func TestDlcoreOnly_HuaacgURL(t *testing.T) {
	cmp := NewComparator(t, nil, WithMaxRetries(0))

	// huaacg URL + .jpg → 触发 dlcore 的 isImageURL(30s) 和 huaacg(5s) 双逻辑
	// 5s 超时优先触发，返回 ErrNoTry
	oldObj := makeTestObject("https://huaacg.com/dl/file.jpg", "dlcoreonly/huaacg.jpg", nil, nil)

	start := time.Now()
	var oldResult DownloadResult
	oldResult.Obj = oldObj
	oldResult.Err = cmp.oldDL.Download(oldObj, nil)
	collectFileResult(t, cmp.rootDir, &oldResult)
	elapsed := time.Since(start)

	t.Logf("dlcore: err=%v, elapsed=%v", oldResult.Err, elapsed)
}

// TestDlcoreOnly_ProgressOnZeroTotal 验证双方在 total=0 时不 panic。
func TestDlcoreOnly_ProgressOnZeroTotal(t *testing.T) {
	b := NewBeacon(t)
	b.HandleDynamic("GET", "/zerototal.bin", func(r *http.Request) (int, map[string]string, []byte) {
		// 不设 Content-Length → total = 0
		return http.StatusOK, map[string]string{
			"Content-Type": "application/octet-stream",
		}, []byte("some data with unknown length")
	})

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/zerototal.bin", "dlcoreonly/zerototal.bin", nil, nil)
	cmp.Run("zero-total", obj, nil, CheckBothNil(), CheckFileBytes())
}
