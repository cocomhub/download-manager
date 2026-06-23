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
