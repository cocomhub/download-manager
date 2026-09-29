// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

// Common API error format strings and constants.
const (
	errFmtInvalidBody     = "Invalid request body: %v"
	hdrContentType        = "Content-Type"
	hdrCacheControl       = "Cache-Control"
	hdrNoCache            = "no-cache"
	errCodeInvalidRequest = "invalid_request"
	errCodeUpdateFailed   = "update_failed"
)

// maxBatchURLs 是批量对象操作（retry_batch/delete_batch/reorder_batch）允许的
// URL 数量上限，防止无鉴权场景下 POST 超大数组造成内存/CPU 放大。
const maxBatchURLs = 500
