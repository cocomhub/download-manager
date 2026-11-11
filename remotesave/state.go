// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"errors"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// objectFileSet 是对象当前本地文件集合（去重、已按任务目录校验）。
type objectFileSet struct {
	files []localFile
	rels  []string
	byRel map[string]localFile
}

func (s *Service) collectObjectFileSet(task core.Task, obj *model.DownloadObject) (objectFileSet, error) {
	files, err := s.objectFiles(task, obj)
	if err != nil {
		return objectFileSet{}, err
	}
	set := objectFileSet{files: files, byRel: make(map[string]localFile, len(files))}
	for _, f := range files {
		rel, rerr := LocalRel(saveDir(task), f.abs)
		if rerr != nil {
			continue
		}
		set.rels = append(set.rels, rel)
		set.byRel[rel] = f
	}
	sort.Strings(set.rels)
	return set, nil
}

// refreshObjectState 仅根据 manifest + 当前文件集合汇总状态，不再触发上传/校验。
// 这是冲突动作后的唯一状态写入口，避免用 force 全量重同步撤销用户选择。
func (s *Service) refreshObjectState(task core.Task, cfg TaskConfig, obj *model.DownloadObject, man *Manifest, fs objectFileSet, retryDelta int) {
	if obj == nil {
		return
	}
	state := obj.GetRemoteSave()
	if state == nil {
		state = &model.RemoteSaveState{}
	}
	relSet := make(map[string]struct{}, len(fs.rels))
	for _, rel := range fs.rels {
		relSet[rel] = struct{}{}
	}
	entries := manifestEntriesForObject(man.Snapshot(), obj.URL)
	for rel := range entries {
		if _, ok := relSet[rel]; !ok {
			relSet[rel] = struct{}{}
			fs.rels = append(fs.rels, rel)
		}
	}
	sort.Strings(fs.rels)

	state.Status = model.RemoteSaveStatusSyncing
	state.Volume, state.Dir = cfg.Volume, cfg.Dir
	state.FilesTotal, state.FilesDone, state.BytesTotal, state.BytesDone = 0, 0, 0, 0
	state.Conflicts, state.LastError = 0, ""
	var failures, skipped, userSkipped, pendingSkipped int
	state.RemotePath = ""
	var bestRemoteSize int64
	preferredSet := false
	preferredRel := ""
	if rel, err := LocalRel(saveDir(task), obj.SavePath); err == nil {
		preferredRel = rel
	}
	for _, rel := range fs.rels {
		f, hasFile := fs.byRel[rel]
		e, ok := entries[rel]
		if !hasFile && !ok {
			continue
		}
		state.FilesTotal++
		if hasFile {
			if st, err := fileStat(f.abs); err == nil {
				state.BytesTotal += st.Size()
			}
		}
		if IsExcluded(rel, cfg.Exclude) {
			skipped++
			pendingSkipped++
			continue
		}
		if !ok {
			// 云端对象可能尚未落本地；本地文件不存在且没有 manifest 记录时不参与状态汇总。
			if hasFile {
				if _, err := fileStat(f.abs); err == nil {
					failures++
					if state.LastError == "" {
						state.LastError = "文件尚未完成同步"
					}
				}
			}
			continue
		}
		switch e.Status {
		case model.RemoteFileStatusUploaded, model.RemoteFileStatusReused:
			state.FilesDone++
			state.BytesDone += e.Size
			if e.RemotePath != "" {
				switch {
				case rel == preferredRel:
					state.RemotePath, bestRemoteSize, preferredSet = e.RemotePath, e.Size, true
				case !preferredSet && (state.RemotePath == "" || e.Size > bestRemoteSize):
					state.RemotePath, bestRemoteSize = e.RemotePath, e.Size
				}
			}
		case model.RemoteFileStatusConflict:
			state.Conflicts++
			if state.LastError == "" {
				state.LastError = e.Reason
			}
		case model.RemoteFileStatusSkipped:
			skipped++
			if e.Reason == "user skip" {
				userSkipped++
			} else {
				pendingSkipped++
			}
		case model.RemoteFileStatusFailed, model.RemoteFileStatusMissing:
			failures++
			if state.LastError == "" {
				state.LastError = e.Reason
			}
		default:
			failures++
		}
	}
	switch {
	case state.Conflicts > 0:
		state.Status = model.RemoteSaveStatusConflict
	case failures > 0:
		state.Status = model.RemoteSaveStatusFailed
	case skipped > 0 && state.FilesDone == 0 && userSkipped > 0 && pendingSkipped == 0:
		state.Status = model.RemoteSaveStatusSkipped
	case skipped > 0:
		state.Status = model.RemoteSaveStatusPending
	default:
		state.Status = model.RemoteSaveStatusSynced
	}
	if state.Status == model.RemoteSaveStatusSynced {
		state.Retries = 0
		if state.RemotePath != "" {
			if play, exp, perr := s.buildPlayURL(state); perr == nil {
				state.PlayURL, state.PlayExpiresAt = play, exp
			}
		} else {
			state.PlayURL, state.PlayExpiresAt = "", time.Time{}
		}
	} else {
		state.PlayURL, state.PlayExpiresAt = "", time.Time{}
		if retryDelta > 0 {
			state.Retries += retryDelta
		}
	}
	state.Files, state.FilesTruncated = ToModelFiles(entries, fs.rels, s.maxFiles())
	state.UpdatedAt = time.Now().UTC()
	s.persistState(task, obj, cfg, state)
}

func fileStat(p string) (os.FileInfo, error) {
	return os.Lstat(p)
}

// manifestEntriesForObject 返回只属于该对象的 manifest 条目（避免 rel 跨对象串用）。
func manifestEntriesForObject(entries map[string]ManifestEntry, objectURL string) map[string]ManifestEntry {
	out := make(map[string]ManifestEntry, len(entries))
	for rel, e := range entries {
		if e.ObjectURL == "" || e.ObjectURL == objectURL {
			out[rel] = e
		}
	}
	return out
}

// remoteRelToTarget 把云端目标路径归一为 manifest 的稳定相对键。
func remoteRelToTarget(cfg TaskConfig, remote string) string {
	if cfg.Dir == "" {
		return strings.TrimPrefix(remote, "/")
	}
	dir := path.Clean(cfg.Dir)
	if strings.HasPrefix(remote, dir+"/") {
		return remote[len(dir)+1:]
	}
	return path.Base(remote)
}

// ConflictInfo 是 manifest 中全部冲突文件（不依赖 mongo 状态截断）。
type ConflictInfo struct {
	ObjectURL    string
	ObjectID     int64
	Path         string
	RemotePath   string
	Size         int64
	SHA256       string
	RemoteSHA256 string
	Reason       string
}

// ListConflicts 返回任务下仍在 storage 中可处理的冲突条目。
func (s *Service) ListConflicts(taskID string) []ConflictInfo {
	task, ok := s.taskByID(taskID)
	if !ok || task == nil {
		return nil
	}
	st := task.Storage()
	if st == nil {
		return nil
	}
	entries := s.manifest(taskID).Snapshot()
	out := make([]ConflictInfo, 0)
	for _, e := range entries {
		if e.Status != model.RemoteFileStatusConflict || e.ObjectURL == "" {
			continue
		}
		obj, err := st.Get(e.ObjectURL)
		if err != nil {
			if errors.Is(err, core.ErrObjectNotFound) {
				continue
			}
			// 存储临时错误不隐藏冲突，交给 resolve 时再报明确错误。
		} else if obj == nil {
			continue
		}
		if obj != nil {
			status := obj.GetStatus()
			if status != model.StatusCompleted && (!obj.IsCloudDownload() || (status != model.StatusFailed && status != model.StatusFailedPermanent)) {
				continue
			}
		}
		var objectID int64
		if obj != nil {
			objectID = obj.GetID()
		}
		out = append(out, ConflictInfo{
			ObjectURL: e.ObjectURL, ObjectID: objectID, Path: e.Path, RemotePath: e.RemotePath, Size: e.Size,
			SHA256: e.SHA256, RemoteSHA256: e.RemoteSHA256, Reason: e.Reason,
		})
	}
	return out
}
