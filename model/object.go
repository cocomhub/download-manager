// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"encoding/json"
	"maps"
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

// Snapshot 返回 DownloadObject 在 RLock 下的深拷贝（Metadata/Extra 均重新分配），
// 供存储层在无锁状态下编码，避免与 soWorker 等其它 goroutine 并发读写 map 触发
// "concurrent map iteration and map write"。返回的副本带独立零值锁，可安全读写。
func (o *DownloadObject) Snapshot() *DownloadObject {
	if o == nil {
		return nil
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	snap := &DownloadObject{
		TaskID:   o.TaskID,
		URL:      o.URL,
		ID:       o.ID,
		SavePath: o.SavePath,
		Status:   o.Status,
		Progress: o.Progress,
	}
	if o.Metadata != nil {
		snap.Metadata = maps.Clone(o.Metadata)
	}
	if o.Extra != nil {
		snap.Extra = deepCopyExtra(o.Extra)
	}
	return snap
}

// deepCopyExtra 深拷贝 Extra 中的可变值（slice / map），标量直接引用。
// 覆盖 Extra 常见的 value 类型：[]string（tags/images）、[]map[string]string、
// []any / []map[string]any（files/links）、map[string]string 等。
//
// 契约：快照必须隔离全部可变引用，否则 marshal 仍会并发读共享的 slice/map。
// 向 Extra 新增可变 value 类型（slice-of-slice、slice-of-struct 等）时，
// 必须在此（及 cloneExtraValue）补对应的深拷贝分支。
func deepCopyExtra(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = cloneExtraValue(v)
	}
	return dst
}

func cloneExtraValue(v any) any {
	switch t := v.(type) {
	case []string:
		return append([]string(nil), t...)
	case []map[string]string:
		out := make([]map[string]string, len(t))
		for i, m := range t {
			out[i] = maps.Clone(m)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(t))
		for i, m := range t {
			out[i] = deepCopyExtra(m)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = cloneExtraValue(e)
		}
		return out
	case map[string]string:
		return maps.Clone(t)
	case map[string]any:
		return deepCopyExtra(t)
	default:
		return v
	}
}
