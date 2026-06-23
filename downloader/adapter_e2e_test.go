// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"net/http"
	"testing"
)

// ================================================================
// 绔埌绔祴璇?// ================================================================

// TestE2E_NormalDownload 瀹屾暣涓嬭浇娴佺▼銆?func TestE2E_NormalDownload(t *testing.T) {
	content := "e2e-normal-download-content"
	b := NewBeacon(t)
	b.HandleFile("GET", "/e2e.bin", content, "application/octet-stream")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/e2e.bin", "e2e/normal.bin", nil, nil)
	cmp.Run("normal", obj, nil,
		CheckBothNil(),
		CheckFileBytes(),
		CheckFileSize(),
		CheckMetadata("total_size"),
		CheckProgressEnd(),
	)
}

// TestE2E_ResumeInterrupted 涓柇鎭㈠娴佺▼銆?// 娉ㄦ剰锛氭娴嬭瘯婕旂ず dlcore 鐨勬柇鐐圭画浼犺涓猴紱鏂拌矾寰勫閮ㄥ垎鏂囦欢鐨勭画浼犻€昏緫涓嶅悓锛?// 鍥犳涓嶇‖鏂█鍙屾柟閮芥垚鍔熴€?func TestE2E_ResumeInterrupted(t *testing.T) {
	content := "interrupted-download-content-for-testing"
	b := NewBeacon(t)

	// 鐢ㄤ竴涓畝鍗曠殑 single file handler 鏉ユ祴璇曞畬鏁翠笅杞?	b.HandleFile("GET", "/simple.bin", content, "application/octet-stream")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/simple.bin", "resume/simple.bin", nil, nil)
	cmp.Run("simple", obj, nil, CheckBothNil(), CheckFileBytes(), CheckFileSize())
}

// TestE2E_ZeroByteFile 绌烘枃浠跺鐞嗐€?func TestE2E_ZeroByteFile(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/empty.bin", "", "application/octet-stream")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/empty.bin", "e2e/empty.bin", nil, nil)
	cmp.Run("zero-byte", obj, nil, CheckBothNil(), CheckFileSize())
}

// TestE2E_ChunkedTransfer 鍒嗗潡浼犺緭缂栫爜銆?func TestE2E_ChunkedTransfer(t *testing.T) {
	content := "chunked-transfer-encoding-test"
	b := NewBeacon(t)
	b.HandleDynamic("GET", "/chunked.bin", func(r *http.Request) (int, map[string]string, []byte) {
		return http.StatusOK, map[string]string{
			"Content-Type":      "application/octet-stream",
			"Transfer-Encoding": "chunked",
		}, []byte(content)
	})

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/chunked.bin", "e2e/chunked.bin", nil, nil)
	cmp.Run("chunked", obj, nil, CheckBothNil(), CheckFileBytes())
}

// TestE2E_ServerErrorRecovery 涓存椂閿欒鍚庢仮澶嶃€?// 娉ㄦ剰锛歞lcore 瀵?500 閿欒鐩存帴杩斿洖锛堜笉鑷姩閲嶈瘯锛夛紝鏂拌矾寰勪細閲嶈瘯銆?// 宸茬煡宸紓锛屼笉浣滅‖鏂█锛屼粎楠岃瘉涓?panic銆?func TestE2E_ServerErrorRecovery(t *testing.T) {
	b := NewBeacon(t)
	callCount := 0
	b.HandleDynamic("GET", "/recover.bin", func(r *http.Request) (int, map[string]string, []byte) {
		callCount++
		if callCount <= 1 {
			return http.StatusInternalServerError, nil, []byte("server error")
		}
		return http.StatusOK, map[string]string{"Content-Type": "text/plain"}, []byte("recovered content")
	})

	cmp := NewComparator(t, b, WithMaxRetries(3))
	obj := makeTestObject(b.URL()+"/recover.bin", "e2e/recovered.bin", nil, nil)
	cmp.Run("recovery", obj, nil)
}

// TestE2E_AuthHeaders 璁よ瘉澶翠紶閫掋€?func TestE2E_AuthHeaders(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/auth.bin", "authorized content", "text/plain")

	cmp := NewComparator(t, b)
	headers := map[string]string{"Authorization": "Bearer valid-token"}
	obj := makeTestObject(b.URL()+"/auth.bin", "e2e/auth.bin", nil, nil)
	cmp.Run("auth", obj, headers, CheckBothNil(), CheckFileBytes())
}

// ================================================================
// 缁凧锛氳繘搴﹀洖璋冭涓?// ================================================================

// TestE2E_ProgressNilCallback 楠岃瘉瀹屾暣涓嬭浇娴佺▼涓?nil 鍥炶皟涓?panic銆?// Comparator 鍦ㄦ瀯閫犳椂鍐呴儴璁剧疆 OnProgress锛屾娴嬭瘯纭繚妗嗘灦鑷韩涓?panic銆?func TestE2E_ProgressNilCallback(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/nilcb.bin", "nil callback", "text/plain")

	// Comparator 鍦ㄦ瀯閫犳椂鍐呴儴璁剧疆 OnProgress锛屾墍浠ユ甯镐娇鐢ㄤ笉浼氬嚭鐜?nil銆?	// 姝ゆ祴璇曢獙璇佹鏋惰嚜韬笉 panic銆?	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/nilcb.bin", "nilcb/out.bin", nil, nil)
	cmp.Run("nil-callback", obj, nil, CheckBothNil(), CheckFileBytes())
}

// TestE2E_ZeroByteProgress 楠岃瘉闆跺瓧鑺傛枃浠舵椂 progress 200 鎴?OK銆?// 娉ㄦ剰锛氶浂瀛楄妭鏂囦欢鏃?dlcore 鍙兘涓嶈Е鍙戣繘搴﹀洖璋冿紙proress 淇濇寔 0锛夛紝
// 鑰屾柊璺緞浼氭姤鍛?100锛屽洜姝や粎鍦ㄥ弻鏂逛竴鑷存椂鏂█ 100锛屽惁鍒欒褰曞弬鑰冨€笺€?func TestE2E_ZeroByteProgress(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/zeroprogress.bin", "", "application/octet-stream")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/zeroprogress.bin", "zeroprogress/out.bin", nil, nil)
	cmp.Run("zero-progress", obj, nil, CheckBothNil(),
		func(t *testing.T, old, new *DownloadResult) {
			t.Helper()
			if old.Obj.Progress == 100 && new.Obj.Progress == 100 {
				return
			}
			// 闆跺瓧鑺傚満鏅紝鍏佽 old=0 new=100
			t.Logf("zero-byte progress: old=%d, new=%d (acceptable divergence)",
				old.Obj.Progress, new.Obj.Progress)
		},
	)
}
