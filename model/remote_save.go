// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import "time"

// RemoteSaveStatus 是对象远程保存的聚合状态。
const (
	RemoteSaveStatusPending  = "pending"
	RemoteSaveStatusSyncing  = "syncing"
	RemoteSaveStatusSynced   = "synced"
	RemoteSaveStatusConflict = "conflict"
	RemoteSaveStatusFailed   = "failed"
	RemoteSaveStatusMissing  = "missing"
	RemoteSaveStatusSkipped  = "skipped"
)

// RemoteFileStatus 是单个文件的远程保存状态。
const (
	RemoteFileStatusUploaded = "uploaded"
	RemoteFileStatusReused   = "reused"
	RemoteFileStatusConflict = "conflict"
	RemoteFileStatusFailed   = "failed"
	RemoteFileStatusMissing  = "missing"
	RemoteFileStatusSkipped  = "skipped"
)

// RemoteFileState 是单个文件在 mongo 中保留的一致性证据。
//
// 用户要求（2026-11-10）：download-manager 内部必须保留每个文件的 sha256，
// 便于核对 sdserver 本地文件与 sproxy 文件是否一致。Typical object 只有主视频 +
// 封面/缩略图/预览，数量很小；超过 MaxFilesPerObject 时只保留前 N 条并置
// FilesTruncated（manifest 仍保留完整明细）。
type RemoteFileState struct {
	Path         string    `json:"path" bson:"path"`                   // 任务内相对路径
	RemotePath   string    `json:"remote_path" bson:"remote_path"`     // 卷内相对路径
	Size         int64     `json:"size" bson:"size"`                   // 本地大小
	MTime        time.Time `json:"mtime" bson:"mtime"`                 // 本地原始 mtime
	SHA256       string    `json:"sha256" bson:"sha256"`               // 本地 sha256（权威）
	RemoteSHA256 string    `json:"remote_sha256" bson:"remote_sha256"` // sproxy 侧 sha256
	Status       string    `json:"status" bson:"status"`               // uploaded/reused/conflict/failed/missing
	Reason       string    `json:"reason,omitempty" bson:"reason,omitempty"`
	UpdatedAt    time.Time `json:"updated_at" bson:"updated_at"`
}

// RemoteSaveState 是对象级远程保存状态（只存聚合 + 逐文件一致性证据，不存账号）。
type RemoteSaveState struct {
	Status         string            `json:"status" bson:"status"`
	Volume         string            `json:"volume" bson:"volume"`
	Dir            string            `json:"dir" bson:"dir"`
	FilesTotal     int               `json:"files_total" bson:"files_total"`
	FilesDone      int               `json:"files_done" bson:"files_done"`
	BytesTotal     int64             `json:"bytes_total" bson:"bytes_total"`
	BytesDone      int64             `json:"bytes_done" bson:"bytes_done"`
	Conflicts      int               `json:"conflicts" bson:"conflicts"`
	Retries        int               `json:"retries,omitempty" bson:"retries,omitempty"`
	FilesTruncated bool              `json:"files_truncated,omitempty" bson:"files_truncated,omitempty"`
	LastError      string            `json:"last_error,omitempty" bson:"last_error,omitempty"`
	RemotePath     string            `json:"remote_path,omitempty" bson:"remote_path,omitempty"` // 主文件（视频）远端路径
	PlayURL        string            `json:"play_url,omitempty" bson:"play_url,omitempty"`
	PlayExpiresAt  time.Time         `json:"play_expires_at" bson:"play_expires_at,omitempty"`
	UpdatedAt      time.Time         `json:"updated_at" bson:"updated_at"`
	Files          []RemoteFileState `json:"files,omitempty" bson:"files,omitempty"`
}

// CloneRemoteSaveState 返回深拷贝（nil 安全）。
func CloneRemoteSaveState(s *RemoteSaveState) *RemoteSaveState {
	if s == nil {
		return nil
	}
	out := *s
	if s.Files != nil {
		out.Files = append([]RemoteFileState(nil), s.Files...)
	}
	return &out
}
