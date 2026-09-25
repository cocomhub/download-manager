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

// Scraper is implemented by tasks that support Manager-driven page scraping.
// Manager's scan loop calls Scrape(ctx) periodically to discover new objects.
// Implementations MUST honor ctx cancellation to allow timely shutdown.
type Scraper interface {
	Scrape(ctx context.Context) error
}

// SmallObjectInfo 描述与主对象关联的小对象（preview/cover/thumb）。
// 小对象由 Manager 统一下载，与主体文件共用相同的下载逻辑（ETag/checksum/重试）。
type SmallObjectInfo struct {
	URL      string `json:"url"`      // 源站 URL
	SavePath string `json:"savePath"` // 本地保存路径
	Rel      string `json:"rel"`      // 关系类型: "cover", "preview", "thumb"
}

// SmallObjectProvider 是可选接口，由有小对象（封面/预览等）的任务实现。
// Task 只负责提供小对象的信息，实际下载由 Manager 的 smallObjectWorker 池完成。
type SmallObjectProvider interface {
	// SmallObjects 返回给定主对象关联的小对象列表。
	SmallObjects(obj *model.DownloadObject) []SmallObjectInfo
}

// ObjectVersioner 是可选接口：任务声明其对象数据结构的当前版本，并提供逐级升级逻辑。
// 管理器启动时自动扫描 version < LatestVersion 的对象，按预设逻辑逐级升级到最新结构。
// 未来的数据结构变更只需：LatestVersion+1、实现对应的 UpgradeStep、新对象构建时写最新 version，
// 不再需要每次新增回填接口。
type ObjectVersioner interface {
	// LatestVersion 返回对象数据结构的当前版本号。version 缺省（0）视为旧数据。
	LatestVersion() int
	// BeginUpgrade 每次升级扫描开始前调用，用于清理跨对象的扫描缓存（如按 book 缓存的重爬结果）。
	BeginUpgrade()
	// UpgradeStep 将对象从当前结构升级到 toVersion（== obj.Version+1，管理器逐级调用直到 LatestVersion）。
	// 返回 modified 表示对象被修改需持久化；实现必须幂等。
	UpgradeStep(obj *model.DownloadObject, toVersion int) (modified bool, err error)
}
