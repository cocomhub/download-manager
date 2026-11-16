// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	sproxyclient "github.com/cocomhub/sproxy/pkg/client"
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
		state.NextRetryAt = time.Time{}
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
			state.NextRetryAt = time.Now().Add(retryBackoff(state.Retries))
		}
	}
	state.Files, state.FilesTruncated = ToModelFiles(entries, fs.rels, s.maxFiles())
	state.UpdatedAt = time.Now().UTC()
	s.persistState(task, obj, cfg, state)
}

// retryBackoff 返回失败重试的指数退避（1m 起，最高 1h）。
func retryBackoff(retries int) time.Duration {
	if retries <= 1 {
		return time.Minute
	}
	if retries > 10 {
		retries = 10
	}
	d := time.Minute << (retries - 1)
	if d > time.Hour {
		return time.Hour
	}
	return d
}

func fileStat(p string) (os.FileInfo, error) {
	return os.Lstat(p)
}

// manifestEntriesForObject 返回只属于该对象的 manifest 条目（避免 rel 跨对象串用）。
func manifestEntriesForObject(entries map[string]ManifestEntry, objectURL string) map[string]ManifestEntry {
	out := make(map[string]ManifestEntry, len(entries))
	for _, e := range entries {
		if e.ObjectURL == "" || e.ObjectURL == objectURL {
			out[e.Path] = e
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
	type objLookup struct {
		obj *model.DownloadObject
		err error
	}
	lookup := map[string]objLookup{}
	for _, e := range entries {
		if e.Status != model.RemoteFileStatusConflict || e.ObjectURL == "" {
			continue
		}
		lr, seen := lookup[e.ObjectURL]
		if !seen {
			lr.obj, lr.err = st.Get(e.ObjectURL)
			lookup[e.ObjectURL] = lr
		}
		obj := lr.obj
		err := lr.err
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

// MigrationItem 是对象目录键变化导致 manifest 远端路径与当前计算路径不一致的条目。
type MigrationItem struct {
	ObjectURL string `json:"object_url"`
	Path      string `json:"path"`
	OldRemote string `json:"old_remote"`
	NewRemote string `json:"new_remote"`
}

// MigrationReport 只生成报告，不移动/删除远端文件。
func (s *Service) MigrationReport(taskID string) ([]MigrationItem, error) {
	task, ok := s.taskByID(taskID)
	if !ok || task == nil {
		return nil, fmt.Errorf("任务不存在: %s", taskID)
	}
	cfg, err := s.taskConfig(task)
	if err != nil || !cfg.Enabled {
		return nil, err
	}
	st := task.Storage()
	if st == nil {
		return nil, fmt.Errorf("任务无存储")
	}
	entries := s.manifest(taskID).Snapshot()
	out := make([]MigrationItem, 0)
	for _, e := range entries {
		if e.ObjectURL == "" || e.RemotePath == "" {
			continue
		}
		obj, gerr := st.Get(e.ObjectURL)
		if gerr != nil || obj == nil || obj.GetStatus() != model.StatusCompleted || obj.IsCloudDownload() {
			continue
		}
		fs, ferr := s.collectObjectFileSet(task, obj)
		if ferr != nil {
			continue
		}
		paths := objectRemotePaths(obj, fs.files, fs.rels, cfg)
		newRemote := paths[e.Path]
		if newRemote == "" || newRemote == e.RemotePath {
			continue
		}
		out = append(out, MigrationItem{ObjectURL: e.ObjectURL, Path: e.Path, OldRemote: e.RemotePath, NewRemote: newRemote})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ObjectURL != out[j].ObjectURL {
			return out[i].ObjectURL < out[j].ObjectURL
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

// MigrationResult 是迁移执行结果。
type MigrationResult struct {
	ObjectURL string `json:"object_url"`
	Path      string `json:"path"`
	OldRemote string `json:"old_remote"`
	NewRemote string `json:"new_remote"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// ApplyMigration 按当前迁移报告执行远端 rename（不删除文件）；目标已存在时 fail-closed。
func (s *Service) ApplyMigration(ctx context.Context, taskID string) ([]MigrationResult, error) {
	task, ok := s.taskByID(taskID)
	if !ok || task == nil {
		return nil, fmt.Errorf("任务不存在: %s", taskID)
	}
	releaseLease, leaseOK := s.acquireTaskLease(task)
	if !leaseOK {
		return nil, fmt.Errorf("任务正被其它实例处理")
	}
	defer releaseLease()
	items, err := s.MigrationReport(taskID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.taskConfig(task)
	if err != nil {
		return nil, err
	}
	client, err := s.opts.ClientFor(s.sproxyCfg(), cfg.Volume)
	if err != nil {
		return nil, err
	}
	man := s.manifest(taskID)
	out := make([]MigrationResult, 0, len(items))
	changed := map[string]*model.DownloadObject{}
	for _, item := range items {
		res := MigrationResult{ObjectURL: item.ObjectURL, Path: item.Path, OldRemote: item.OldRemote, NewRemote: item.NewRemote}
		_, serr := client.Stat(ctx, item.OldRemote)
		if serr != nil && !errors.Is(serr, sproxyclient.ErrNotFound) {
			res.Error = serr.Error()
			out = append(out, res)
			continue
		}
		if errors.Is(serr, sproxyclient.ErrNotFound) {
			if _, nerr := client.Stat(ctx, item.NewRemote); nerr != nil {
				res.Error = fmt.Sprintf("旧/新远端均不存在: %v / %v", serr, nerr)
				out = append(out, res)
				continue
			}
			res.Success = true
		} else {
			if _, nerr := client.Stat(ctx, item.NewRemote); nerr == nil {
				res.Error = "目标已存在，拒绝覆盖: " + item.NewRemote
				out = append(out, res)
				continue
			} else if !errors.Is(nerr, sproxyclient.ErrNotFound) {
				res.Error = "检查目标失败: " + nerr.Error()
				out = append(out, res)
				continue
			}
			if rerr := client.Rename(ctx, item.OldRemote, item.NewRemote); rerr != nil {
				res.Error = rerr.Error()
				out = append(out, res)
				continue
			}
			res.Success = true
		}
		if e, ok := man.GetFor(item.ObjectURL, item.Path); ok {
			e.RemotePath = item.NewRemote
			man.Put(e)
		}
		out = append(out, res)
		if obj, gerr := task.Storage().Get(item.ObjectURL); gerr == nil && obj != nil {
			changed[item.ObjectURL] = obj
		}
	}
	_ = man.Flush()
	for _, obj := range changed {
		unlock := s.lockObject(obj.URL)
		fs, ferr := s.collectObjectFileSet(task, obj)
		if ferr == nil {
			s.refreshObjectState(task, cfg, obj, man, fs, 0)
		}
		unlock()
	}
	return out, nil
}
