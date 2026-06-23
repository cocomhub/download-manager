// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "io"

// DownloadHint 鎼哄甫涓嬭浇鎻愮ず淇℃伅锛屽府鍔?Selector 鍜?Extractor 鍋氬嚭鍐崇瓥銆?type DownloadHint struct {
	FileSize    int64
	ContentType string
	Extractor   string
	Tags        map[string]string
}

// Request 鎻忚堪涓€涓笅杞借姹傦紝鍖呭惈鐩爣 URL銆佷繚瀛樿矾寰勩€佸ご淇℃伅銆佽繘搴﹀洖璋冪瓑銆?type Request struct {
	URL           string
	SavePath      string
	Headers       map[string]string
	TrackProgress bool
	OnProgress    func(progress float64, downloaded, total int64)
	// OnMetadata 鍦ㄦ瘡娆?Extractor 鎻愬彇鍒板厓鏁版嵁锛圗Tag銆乧hecksum 绛夛級鏃惰Е鍙戙€?	// 瑙﹀彂鏃?req.Metadata[key] 宸茶璁剧疆锛屽洖璋冪敤浜庢帴鏀舵柟绔嬪嵆鎸佷箙鍖栵紝閬垮厤 crash 涓㈠け銆?	OnMetadata func(key, value string)
	Metadata   map[string]string
	Hint       *DownloadHint
	Result     *DownloadResult // Extractor 濉厖姝ゅ瓧娈碉紝璋冪敤鏂硅鍙栧悗鏄惧紡搴旂敤鍒扮洰鏍囧璞?}

// DownloadResult 鍖呭惈涓嬭浇瀹屾垚鍚庣殑鍏冩暟鎹俊鎭€?// Extractor 鍦ㄤ笅杞芥垚鍔熷悗濉厖姝ょ粨鏋勪綋锛屼笉鍐嶇洿鎺ュ啓鍏?Request.Metadata銆?type DownloadResult struct {
	StatusCode    int
	ContentLength int64
	TotalSize     int64
	MD5Base64     string
	MD5Hex        string
	ModTime       string // RFC3339Nano 鏍煎紡
}

// RangeRequest 鎻忚堪涓€涓?HTTP Range 璇锋眰鐨勫亸绉婚噺銆?type RangeRequest struct {
	Offset int64
}

// TransportRequest 鏄?Transport 灞備娇鐢ㄧ殑瀹屾暣璇锋眰鎻忚堪銆?type TransportRequest struct {
	URL      string
	Method   string
	Headers  map[string]string
	Body     []byte
	Range    *RangeRequest
	ProxyURL string
}

// TransportResponse 鏄?Transport 灞傝繑鍥炵殑鍝嶅簲锛屽寘鍚?Body 鍜屽厓鏁版嵁銆?type TransportResponse struct {
	Body          io.ReadCloser
	StatusCode    int
	ContentLength int64
	Headers       map[string]string
	ProxyURL      string
}
