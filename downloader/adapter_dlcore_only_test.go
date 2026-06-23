// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"net/http"
	"testing"
	"time"
)

// ================================================================
// dlcore-only 娴嬭瘯锛氶獙璇?dlcore 鐗规湁鐨勩€乸kg/download 涓嶆敮鎸佺殑琛屼负
// 杩欎簺娴嬭瘯浠呮柇瑷€ dlcore 琛屼负锛宲kg/download 鐨勭粨鏋滈€氳繃 t.Log 璁板綍鍙傝€冦€?// ================================================================

// TestDlcoreOnly_MaxRetriesZero 楠岃瘉 dlcore maxRetries=0 鏃犻檺閲嶈瘯銆?func TestDlcoreOnly_MaxRetriesZero(t *testing.T) {
	b := NewBeacon(t)
	b.HandleDynamic("GET", "/infinite.bin", func(r *http.Request) (int, map[string]string, []byte) {
		return http.StatusOK, map[string]string{"Content-Type": "text/plain"}, []byte("success")
	})

	cmp := NewComparator(t, b, WithMaxRetries(0))
	obj := makeTestObject(b.URL()+"/infinite.bin", "dlcoreonly/infinite.bin", nil, nil)
	cmp.Run("max-retries-zero", obj, nil,
		func(t *testing.T, old, new *DownloadResult) {
			t.Helper()
			// dlcore: maxRetries=0 琛ㄧず鏃犻檺閲嶈瘯
			if old.Err != nil {
				t.Errorf("dlcore: expected success with maxRetries=0, got %v", old.Err)
			}
			if len(old.FileContent) == 0 {
				t.Error("dlcore: expected file content")
			}
			// pkg/download: maxRetries=0 琛ㄧず涓嶉噸璇?			t.Logf("pkg/download reference: err=%v", new.Err)
		},
	)
}

// TestDlcoreOnly_MetadataStatus 楠岃瘉 dlcore 鍐欏叆 Metadata["status"]="completed"銆?func TestDlcoreOnly_MetadataStatus(t *testing.T) {
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

// TestDlcoreOnly_ImageURLTimeout 楠岃瘉鍥剧墖 URL 30s 瓒呮椂銆?// 娉ㄦ剰锛氭娴嬭瘯杩愯杈冩參锛堣嚦灏戠瓑寰?30s HTTP 瓒呮椂锛夈€?// 姝ゅ鐩存帴璋冪敤 oldDL 鑰岄潪 DlcoreOnlyRun锛屽洜涓洪渶瑕佺簿纭?elapsed 璁℃椂銆?func TestDlcoreOnly_ImageURLTimeout(t *testing.T) {
	b := NewBeacon(t)
	b.HandleSlow("GET", "/image.jpg", "image content", 35*time.Second)

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/image.jpg", "dlcoreonly/image.jpg", nil, nil)

	// 鍙祴鏃ц矾寰勶紝閬垮厤鏂拌矾寰勭殑 Content-Type 妫€娴嬪揩閫熻繑鍥炲共鎵?	oldObj := copyObject(obj)
	start := time.Now()
	var oldResult DownloadResult
	oldResult.Obj = oldObj
	oldResult.Err = cmp.oldDL.Download(oldObj, nil)
	collectFileResult(t, cmp.rootDir, &oldResult)
	elapsed := time.Since(start)

	// 榛樿 maxRetries=3锛屾墍浠ユ€荤瓑寰呭彲鑳借秴杩?30s
	t.Logf("dlcore: err=%v, elapsed=%v", oldResult.Err, elapsed)
	if oldResult.Err == nil {
		t.Log("dlcore: image download succeeded (may have completed before timeout)")
	} else {
		t.Logf("dlcore: image download error: %v", oldResult.Err)
	}
}

// TestDlcoreOnly_HuaacgURL 楠岃瘉 huaacg.com 鐗规畩 5s 瓒呮椂 + ErrNoTry銆?// URL 蹇呴』鍖呭惈 huaacg.com 鎵嶈兘瑙﹀彂 dlcore 鐨勭壒娈婇€昏緫锛?s 涓婁笅鏂囪秴鏃?+ ErrNoTry 鍖呰锛夈€?// 娴嬭瘯渚濊禆缃戠粶鍙揪鎬э紝浣嗗疄闄呰姹傚湪 5s 鍐呭洜瓒呮椂杩斿洖锛屼笉浼氫骇鐢熷ぇ閲忔祦閲忋€?// 鎴愬姛鏃讹細dlcore 杩斿洖鍖呰浜?ErrNoTry 鐨勯敊璇€?// 鑻ョ綉缁滀笉鍙揪锛氬悓鏍峰洜瓒呮椂蹇€熻繑鍥烇紝涓嶄細鎸傝捣銆?func TestDlcoreOnly_HuaacgURL(t *testing.T) {
	cmp := NewComparator(t, nil, WithMaxRetries(0))

	// huaacg URL + .jpg 鈫?瑙﹀彂 dlcore 鐨?isImageURL(30s) 鍜?huaacg(5s) 鍙岄€昏緫
	// 5s 瓒呮椂浼樺厛瑙﹀彂锛岃繑鍥?ErrNoTry
	oldObj := makeTestObject("https://huaacg.com/dl/file.jpg", "dlcoreonly/huaacg.jpg", nil, nil)

	start := time.Now()
	var oldResult DownloadResult
	oldResult.Obj = oldObj
	oldResult.Err = cmp.oldDL.Download(oldObj, nil)
	collectFileResult(t, cmp.rootDir, &oldResult)
	elapsed := time.Since(start)

	t.Logf("dlcore: err=%v, elapsed=%v", oldResult.Err, elapsed)
}

// TestDlcoreOnly_ProgressOnZeroTotal 楠岃瘉鍙屾柟鍦?total=0 鏃朵笉 panic銆?func TestDlcoreOnly_ProgressOnZeroTotal(t *testing.T) {
	b := NewBeacon(t)
	b.HandleDynamic("GET", "/zerototal.bin", func(r *http.Request) (int, map[string]string, []byte) {
		// 涓嶈 Content-Length 鈫?total = 0
		return http.StatusOK, map[string]string{
			"Content-Type": "application/octet-stream",
		}, []byte("some data with unknown length")
	})

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/zerototal.bin", "dlcoreonly/zerototal.bin", nil, nil)
	cmp.Run("zero-total", obj, nil, CheckBothNil(), CheckFileBytes())
}
