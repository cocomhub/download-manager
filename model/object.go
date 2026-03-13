// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

// DownloadObject 代表一个具体的下载对象
type DownloadObject struct {
	TaskID   string            `json:"task_id,omitempty" bson:"task_id,omitempty"`
	URL      string            `json:"url" bson:"url"`
	SavePath string            `json:"save_path" bson:"save_path"`
	Metadata map[string]string `json:"metadata" bson:"metadata"`
	Extra    map[string]any    `json:"extra" bson:"extra"`
	Status   string            `json:"status" bson:"status"` // pending, downloading, completed, failed
	Progress int               `json:"progress" bson:"progress"`
}
