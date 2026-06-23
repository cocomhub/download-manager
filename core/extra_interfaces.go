// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	"github.com/cocomhub/download-manager/model"
)

// PathStrategy defines how save paths are resolved for download objects.
type PathStrategy interface {
	Resolve(baseDir string, taskID string, title string, fileType string) (dir string, filename string)
}

// ScrapeCap is implemented by tasks that support Manager-driven page scraping.
// Manager's scan loop calls Scrape(ctx) periodically to discover new objects.
// Implementations MUST honor ctx cancellation to allow timely shutdown.
type ScrapeCap interface {
	Scrape(ctx context.Context) error
}

// SmallObjectInfo 描述与主对象关联的小对象（preview/cover/thumb）。
// 小对象由 Manager 统一下载，与主体文件共用相同的下载逻辑（ETag/checksum/重试）。
type SmallObjectInfo struct {
	URL      string `json:"url"`      // 源站 URL
	SavePath string `json:"savePath"` // 本地保存路径
	Rel      string `json:"rel"`      // 关系类型: "cover", "preview", "thumb"
}

// SmallObjectCap 是可选接口，由有小对象（封面/预览等）的任务实现。
// Task 只负责提供小对象的信息，实际下载由 Manager 的 smallObjectWorker 池完成。
type SmallObjectCap interface {
	// SmallObjects 返回给定主对象关联的小对象列表。
	SmallObjects(obj *model.DownloadObject) []SmallObjectInfo
}
