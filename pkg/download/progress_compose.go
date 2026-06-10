// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

// ComposeProgress 将多个进度回调组合为一个。
// 组合后的回调会按顺序调用所有传入的非 nil 回调。
// 如果所有回调均为 nil 或切片为空，返回 nil。
//
// 用法：
//
//	req.OnProgress = ComposeProgress(
//	    obj.SetProgress,
//	    NewProgressLogCallback(WithLogWriter(logFile)),
//	)
func ComposeProgress(cbs ...func(float64, int64, int64)) func(float64, int64, int64) {
	// 过滤掉 nil 回调
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
