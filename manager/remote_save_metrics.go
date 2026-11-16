// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"

	"github.com/cocomhub/download-manager/model"
)

// RemoteSaveTaskMetrics 是任务级 remote-save 指标（按对象持久化状态聚合）。
type RemoteSaveTaskMetrics struct {
	ObjectsTotal          int   `json:"objects_total"`
	ObjectsSynced         int   `json:"objects_synced"`
	ObjectsConflict       int   `json:"objects_conflict"`
	ObjectsFailed         int   `json:"objects_failed"`
	FilesUploaded         int   `json:"files_uploaded"`
	FilesReused           int   `json:"files_reused"`
	FilesFailed           int   `json:"files_failed"`
	FilesSkipped          int   `json:"files_skipped"`
	Conflicts             int   `json:"conflicts"`
	BytesDone             int64 `json:"bytes_done"`
	BytesTotal            int64 `json:"bytes_total"`
	FilesTruncatedObjects int   `json:"files_truncated_objects"`
}

// RemoteSaveMetrics 返回任务级 remote-save 指标。
func (m *Manager) RemoteSaveMetrics(taskID string) (RemoteSaveTaskMetrics, error) {
	t, ok := m.getTask(taskID)
	if !ok {
		return RemoteSaveTaskMetrics{}, fmt.Errorf("任务不存在: %s", taskID)
	}
	objs, err := m.listRemoteSaveObjects(t)
	if err != nil {
		return RemoteSaveTaskMetrics{}, err
	}
	var out RemoteSaveTaskMetrics
	for _, obj := range objs {
		rs := obj.GetRemoteSave()
		if rs == nil {
			continue
		}
		out.ObjectsTotal++
		switch rs.Status {
		case model.RemoteSaveStatusSynced:
			out.ObjectsSynced++
		case model.RemoteSaveStatusConflict:
			out.ObjectsConflict++
		case model.RemoteSaveStatusFailed, model.RemoteSaveStatusMissing:
			out.ObjectsFailed++
		}
		uploaded, reused, failed, skipped := rs.FilesUploaded, rs.FilesReused, rs.FilesFailed, rs.FilesSkipped
		if uploaded+reused+failed+skipped == 0 {
			for _, f := range rs.Files {
				switch f.Status {
				case model.RemoteFileStatusUploaded:
					uploaded++
				case model.RemoteFileStatusReused:
					reused++
				case model.RemoteFileStatusFailed, model.RemoteFileStatusMissing:
					failed++
				case model.RemoteFileStatusSkipped:
					skipped++
				}
			}
		}
		out.FilesUploaded += uploaded
		out.FilesReused += reused
		out.FilesFailed += failed
		out.FilesSkipped += skipped
		out.Conflicts += rs.Conflicts
		out.BytesDone += rs.BytesDone
		out.BytesTotal += rs.BytesTotal
		if rs.FilesTruncated {
			out.FilesTruncatedObjects++
		}
	}
	return out, nil
}
