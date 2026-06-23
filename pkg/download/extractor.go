// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "context"

// Extractor 鎺ュ彛璐熻矗鏍规嵁 URL 鍜岃姹備俊鎭彁鍙栧嚭鏈€缁堢殑鍙笅杞借祫婧愩€?// 涓嶅悓鐨勫疄鐜板搴斾笉鍚岀殑鎻愬彇绛栫暐锛堝鍘熺敓鐩撮摼銆乻craper銆乵3u8 瑙ｆ瀽绛夛級銆?type Extractor interface {
	// Name 杩斿洖鎻愬彇鍣ㄧ殑鍚嶇О銆?	Name() string

	// Match 鍒ゆ柇璇ユ彁鍙栧櫒鏄惁鑳藉澶勭悊缁欏畾鐨?URL銆?	Match(ctx context.Context, url string) bool

	// Extract 瀵硅姹傝繘琛屾彁鍙栧鐞嗭紝鍙兘浼氫慨鏀?req 鐨勫瓧娈碉紙濡?URL銆丠eaders锛夈€?	Extract(ctx context.Context, req *Request) error
}

// Canceller 琛ㄧず鏀寔鍙栨秷姝ｅ湪杩涜鐨勪笅杞界殑 Extractor銆?type Canceller interface {
	// Cancel 鍙栨秷鎸囧畾 URL 鐨勪笅杞姐€?	Cancel(url string) error
}
