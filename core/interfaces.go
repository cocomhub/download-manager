package core

import (
	"download-manager/model"
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
	Search(filter interface{}) ([]*model.DownloadObject, error)
}

// Task 定义下载任务的行为
type Task interface {
	// ID 返回任务唯一标识
	ID() string
	// GetDownloadObjects 获取该任务当前需要下载的对象列表
	GetDownloadObjects() ([]*model.DownloadObject, error)
	// UpdateStatus 更新下载对象的状态
	UpdateStatus(obj *model.DownloadObject, status string, err error) error
	// Type 返回任务类型
	Type() string
	// Close 关闭任务，执行清理或持久化操作
	Close() error
}

// Downloader 定义下载器的行为
type Downloader interface {
	// Download 执行下载
	Download(obj *model.DownloadObject) error
	// Name 返回下载器名称
	Name() string
}
