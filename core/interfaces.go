package core

import "download-manager/model"

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
}

// Downloader 定义下载器的行为
type Downloader interface {
	// Download 执行下载
	Download(obj *model.DownloadObject) error
	// Name 返回下载器名称
	Name() string
}
