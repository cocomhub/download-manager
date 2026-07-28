// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package ui registers {{TYPE}} custom UI assets via the TaskUIAssets framework.
//
// 使用方式：将本文件复制到 task/<your-type>/ui/ui.go，替换 {{TYPE}} 和 {{LABEL}}。
// 同时创建 task/<your-type>/ui/assets/viewer.js 编写 UI 插件代码。
package ui

import (
	"embed"

	"github.com/cocomhub/download-manager/core"
)

//go:embed assets/viewer.js
var assets embed.FS

func init() {
	core.RegisterTaskUI("{{TYPE}}", core.TaskUIAssets{
		FS:      assets,
		JSPaths: []string{"assets/viewer.js"},
		Label:   "{{LABEL}}",
		// HasForm: true,      // 是否有扩展表单（新建任务弹窗中的额外字段）
		// HasViewer: true,    // 是否有自定义查看器（点击对象时弹窗）
		// HasAggregate: true, // 是否有聚合视图
	})
}