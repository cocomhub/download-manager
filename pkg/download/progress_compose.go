// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

// ComposeProgress 灏嗗涓繘搴﹀洖璋冪粍鍚堜负涓€涓€?// 缁勫悎鍚庣殑鍥炶皟浼氭寜椤哄簭璋冪敤鎵€鏈変紶鍏ョ殑闈?nil 鍥炶皟銆?// 濡傛灉鎵€鏈夊洖璋冨潎涓?nil 鎴栧垏鐗囦负绌猴紝杩斿洖 nil銆?//
// 鐢ㄦ硶锛?//
//	req.OnProgress = ComposeProgress(
//	    obj.SetProgress,
//	    NewProgressLogCallback(WithLogWriter(logFile)),
//	)
func ComposeProgress(cbs ...func(float64, int64, int64)) func(float64, int64, int64) {
	// 杩囨护鎺?nil 鍥炶皟
	valid := make([]func(float64, int64, int64), 0, len(cbs))
	for _, cb := range cbs {
		if cb != nil {
			valid = append(valid, cb)
		}
	}
	switch len(valid) {
	case 0:
		return nil
	case 1:
		return valid[0]
	default:
		return func(progress float64, downloaded, total int64) {
			for _, cb := range valid {
				cb(progress, downloaded, total)
			}
		}
	}
}
