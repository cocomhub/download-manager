// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "io"

// ProgressReader 鍖呰 io.Reader锛屽湪璇诲彇杩囩▼涓Е鍙戣繘搴﹀洖璋冦€?// 閫傜敤浜庝笅杞藉満鏅腑瀹炴椂鎶ュ憡宸茶鍙栧瓧鑺傛暟鍗犳€诲瓧鑺傛暟鐨勬瘮渚嬨€?type ProgressReader struct {
	reader     io.Reader
	total      int64
	downloaded int64
	onProgress func(progress float64, downloaded, total int64)
}

// NewProgressReader 鍒涘缓涓€涓?ProgressReader銆?// total 涓洪鏈熸€诲瓧鑺傛暟锛? 琛ㄧず鏈煡锛屾鏃朵笉瑙﹀彂鍥炶皟锛夛紱onProgress 涓鸿繘搴﹀洖璋冿紙鍙负 nil锛夈€?func NewProgressReader(reader io.Reader, total int64, onProgress func(float64, int64, int64)) *ProgressReader {
	return &ProgressReader{
		reader:     reader,
		total:      total,
		onProgress: onProgress,
	}
}

// Read 瀹炵幇 io.Reader銆傛瘡娆¤鍙栧悗鏇存柊杩涘害骞跺洖璋冦€?func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.downloaded += int64(n)
		if pr.total > 0 && pr.onProgress != nil {
			progress := float64(pr.downloaded) / float64(pr.total) * 100
			pr.onProgress(progress, pr.downloaded, pr.total)
		}
	}
	return n, err
}

// Done 鏍囪璇诲彇瀹屾垚锛屽己鍒惰缃繘搴︿负 100%銆?func (pr *ProgressReader) Done() {
	if pr.onProgress != nil && pr.total > 0 {
		pr.onProgress(100, pr.total, pr.total)
	}
}
