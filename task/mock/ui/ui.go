// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package mockui 注册 mock 任务 UI 资产（视频查看器，模拟视频任务能力）。
package mockui

import (
	"embed"

	"github.com/cocomhub/download-manager/core"
)

//go:embed assets/viewer.js
var uiFS embed.FS

const taskType = "mock"

func init() {
	core.RegisterTaskUI(taskType, core.TaskUIAssets{
		FS:        uiFS,
		JSPaths:   []string{"assets/viewer.js"},
		Label:     "模拟",
		HasViewer: true,
	})
}
