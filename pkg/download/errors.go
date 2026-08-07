// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "errors"

// ErrNoTry 表示没有重试次数剩余，下载应终止。
var ErrNoTry = errors.New("no try left")

// ErrAlreadyDownloading 表示同一 URL 的下载请求正在被处理中。
var ErrAlreadyDownloading = errors.New("already downloading")

// IsNoTry 判断错误是否为 ErrNoTry 或其包装。
func IsNoTry(err error) bool {
	return errors.Is(err, ErrNoTry)
}
