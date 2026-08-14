// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"encoding/json"
	"sync"
)

// DownloadObject 代表一个具体的下载对象
type DownloadObject struct {
	TaskID   string            `json:"task_id,omitempty" bson:"task_id,omitempty"`
	URL      string            `json:"url" bson:"url"`
	ID       int64             `json:"id,omitempty" bson:"id,omitempty"`
	SavePath string            `json:"save_path" bson:"save_path"`
	Metadata map[string]string `json:"metadata" bson:"metadata"`
	Extra    map[string]any    `json:"extra" bson:"extra"`
	Status   string            `json:"status" bson:"status"`
	Progress int               `json:"progress" bson:"progress"`
	// Version 对象数据结构版本（通用升级机制用）：version < 任务 LatestVersion 的对象
	// 会在启动标准化时被 ObjectVersioner 自动逐级升级到最新结构。缺省 0 视为旧数据。
	Version int64 `json:"version,omitempty" bson:"version,omitempty"`

	mu sync.RWMutex `json:"-" bson:"-"`
}

func (o *DownloadObject) GetID() int64 {
	if o == nil {
		return 0
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.ID
}

func (o *DownloadObject) SetID(id int64) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.ID = id
}

func (o *DownloadObject) GetProgress() int {
	if o == nil {
		return 0
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.Progress
}

func (o *DownloadObject) SetProgress(p int) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Progress = p
}

func (o *DownloadObject) GetStatus() string {
	if o == nil {
		return ""
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.Status
}

func (o *DownloadObject) SetStatus(s string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Status = s
}

func (o *DownloadObject) GetVersion() int64 {
	if o == nil {
		return 0
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.Version
}

func (o *DownloadObject) SetVersion(v int64) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Version = v
}

// MarshalJSON preserves backward-compatible JSON output.
func (o *DownloadObject) MarshalJSON() ([]byte, error) {
	if o == nil {
		return json.Marshal(nil)
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	type Alias DownloadObject
	return json.Marshal((*Alias)(o))
}

// mu returns the mutex for external synchronization of Extra and Metadata.
func (o *DownloadObject) Lock()    { o.mu.Lock() }
func (o *DownloadObject) Unlock()  { o.mu.Unlock() }
func (o *DownloadObject) RLock()   { o.mu.RLock() }
func (o *DownloadObject) RUnlock() { o.mu.RUnlock() }
