// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"github.com/cocomhub/download-manager/model"
)

// Storage 定义下载状态存储的行为
type Storage interface {
	// Get 获取单个对象状态
	Get(id string) (*model.DownloadObject, error)
	// Update 更新单个对象的状态
	Update(obj *model.DownloadObject) error
	// Delete 删除对象状态
	Delete(id string) error
	// Search 搜索对象 (简单起见，返回所有或根据filter返回)
	// 在本系统中，通常用于获取该Task下的所有对象状态
	Search(filter any) ([]*model.DownloadObject, error)
}

// Task 定义下载任务的行为
type Task interface {
	// ID 返回任务唯一标识
	ID() string
	// GetDownloadHeaders 获取下载对象的自定义HTTP头
	GetDownloadHeaders() map[string]string
	// GetDownloadObjects 获取该任务当前需要下载的对象列表
	GetDownloadObjects() ([]*model.DownloadObject, error)
	// UpdateStatus 更新下载对象的状态
	UpdateStatus(obj *model.DownloadObject, status string, err error) error
	// Type 返回任务类型
	Type() string
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

// DownloaderSetter 任务可实现该接口以接收下载器
type DownloaderSetter interface {
	SetDownloader(d Downloader)
}

// CacheLoader 任务可实现该接口以加载缓存
type CacheLoader interface {
	LoadCache() error
}

// CacheSaver 任务可实现该接口以保存缓存
type CacheSaver interface {
	SaveCache() error
}

// EventType 定义事件类型
type EventType string

const (
	EventTaskUpdate         EventType = "task_update"          // 任务统计更新 (单任务)
	EventTaskListChange     EventType = "task_list_change"     // 任务列表变动 (添加/删除任务)
	EventObjectUpdate       EventType = "object_update"        // 对象状态/进度更新
	EventSharedObjectUpdate EventType = "shared_object_update" // 共享对象状态更新
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
