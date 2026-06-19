// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"log/slog"

	"github.com/cocomhub/download-manager/model"
)

type StorageFilter struct {
	TaskIDs  []string
	URLs     []string
	Statuses []string
	Metadata map[string]string
	Search   string
}

type StorageSort struct {
	Field string
	Desc  bool
}

type StorageQuery struct {
	Filter StorageFilter
	Sort   []StorageSort
	Offset int64
	Limit  int64
}

// Storage 定义下载状态存储的行为
type Storage interface {
	// Get 获取单个对象状态
	Get(id string) (*model.DownloadObject, error)
	// Update 更新单个对象的状态
	Update(obj *model.DownloadObject) error
	// Delete 删除对象状态
	Delete(id string) error
	// Search 按查询条件搜索对象。
	// nil 查询表示不过滤、不排序、不分页，返回当前存储中的全部对象。
	Search(query *StorageQuery) ([]*model.DownloadObject, error)
	// Count 返回匹配查询过滤条件的对象总数，忽略排序与分页参数。
	Count(query *StorageQuery) (int64, error)
	// Exists 批量检查给定对象 ID 是否存在。
	Exists(ids []string) (map[string]bool, error)
}

// Task 定义下载任务的行为
type Task interface {
	// ID 返回任务唯一标识
	ID() string
	// Type 返回任务类型
	Type() string
	// Logger 日志
	Logger() *slog.Logger
	// Storage 返回任务的存储后端
	Storage() Storage
	// SetDownloader 设置下载器
	SetDownloader(d Downloader)
	// GetDownloadHeaders 获取下载对象的自定义HTTP头
	GetDownloadHeaders() map[string]string
	// GetDownloadObjects 获取该任务当前需要下载的对象列表
	GetDownloadObjects() ([]*model.DownloadObject, error)
	// UpdateStatus 更新下载对象的状态
	UpdateStatus(obj *model.DownloadObject, status string, err error) error
	// Concurrency 并发数
	Concurrency() int
	// SetConcurrency 设置并发数
	SetConcurrency(int) error
	// RefreshInterval 刷新时间
	RefreshInterval() int
	// SetRefreshInterval 设置刷新时间
	SetRefreshInterval(int) error
	// Start 开始任务
	Start() error
	// ResolveObject 解析对象详情，填充 Extra["files"]（主要下载项列表）。
	// 无需 resolve 的 task（如 urllist）直接返回 nil。
	// ctx 用于超时控制。
	ResolveObject(ctx context.Context, obj *model.DownloadObject) error
	// Close 关闭任务，执行清理或持久化操作
	Close() error
}

type FailedTask interface {
	// MarkAsFailed 标记任务为失败状态
	MarkAsFailed(obj *model.DownloadObject, err error)
}

// Downloader 定义下载器的行为
type Downloader interface {
	// Download 执行下载
	Download(obj *model.DownloadObject, headers map[string]string) error
	// Name 返回下载器名称
	Name() string
}

// DownloaderWithContext 表示支持上下文注入的下载器。
type DownloaderWithContext interface {
	SetContext(ctx context.Context)
}

// DownloaderWithDomainLimits 表示支持域名并发限制的下载器。
type DownloaderWithDomainLimits interface {
	ApplyDomainLimits(limits map[string]int)
}

// DownloaderWithMetrics 表示支持暴露下载指标的下载器。
type DownloaderWithMetrics interface {
	MetricsRegistry() any // returns *download.MetricRegistry or similar
}

// SharedRegistry 用于跨任务共享基于 URL 的对象状态
type SharedRegistry interface {
	Get(url string) (*model.DownloadObject, error)
	Update(obj *model.DownloadObject) error
	Delete(url string) error
}

// SharedRegistrySetter 任务可实现该接口以接收共享注册表
type SharedRegistrySetter interface {
	SetSharedRegistry(reg SharedRegistry)
}

// TaskStatusGuarder 任务可实现该接口以支持原子化的状态守卫更新。
// SetStatusUnlessCancelled 在 b.mu 保护下检查对象未被取消后更新状态，
// 用于解决 processResolve 与 CancelObject 之间的 TOCTOU 竞争。
// 返回 true 表示状态已更新，false 表示对象已被取消（跳过更新）。
type TaskStatusGuarder interface {
	SetStatusUnlessCancelled(obj *model.DownloadObject, status string, err error) bool
}

// EventType 定义事件类型
type EventType string

const (
	EventTaskUpdate         EventType = "task_update"          // 任务统计更新 (单任务)
	EventTaskListChange     EventType = "task_list_change"     // 任务列表变动 (添加/删除任务)
	EventObjectUpdate       EventType = "object_update"        // 对象状态/进度更新
	EventSharedObjectUpdate EventType = "shared_object_update" // 共享对象状态更新
	EventProgressBatch      EventType = "progress_batch"       // 批量进度广播
)

// Event 系统事件
type Event struct {
	Type    EventType `json:"type"`
	Payload any       `json:"payload"`
}

// EventBus 定义事件总线行为
type EventBus interface {
	Subscribe() <-chan Event
	Unsubscribe(ch <-chan Event)
}
