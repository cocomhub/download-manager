// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

// 下载对象状态常量
const (
	StatusPending         = "pending"
	StatusResolving       = "resolving"
	StatusDownloading     = "downloading"
	StatusCompleted       = "completed"
	StatusFailed          = "failed"
	StatusFailedPermanent = "failed_permanent"
	StatusCancelled       = "cancelled"
)
