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
	SavePath string            `json:"save_path" bson:"save_path"`
	Metadata map[string]string `json:"metadata" bson:"metadata"`
	Extra    map[string]any    `json:"extra" bson:"extra"`
	Status   string            `json:"status" bson:"status"` // pending, downloading, completed, failed
	Progress int               `json:"progress" bson:"progress"`

	mu sync.RWMutex // protects Status and Progress for concurrent access
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

// MarshalJSON preserves backward-compatible JSON output for the exported fields.
// Custom marshal is not strictly needed since json encoder reads the struct fields,
// but we keep it explicit to ensure safety.
func (o *DownloadObject) MarshalJSON() ([]byte, error) {
	// Use type alias to avoid infinite recursion
	type Alias DownloadObject
	return json.Marshal((*Alias)(o))
}
