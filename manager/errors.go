// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import "errors"

// errTaskNotFound is a sentinel error returned when a task is not found
// in the manager's task registry. Callers should use errors.Is() to check.
var errTaskNotFound = errors.New("task not found")

// metaJSONSuffix is the file extension suffix for backup metadata files.
const metaJSONSuffix = ".meta.json"

// ErrTaskNotFound / ErrObjectNotFound 导出哨兵：供 API 层区分「客户端错误」与「存储故障」。
var (
	ErrTaskNotFound   = errTaskNotFound
	ErrObjectNotFound = errObjectNotFound
)

// errObjectNotFound 表示任务下未找到指定 URL 的对象。
var errObjectNotFound = errors.New("object not found")
