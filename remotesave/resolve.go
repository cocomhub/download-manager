// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// 冲突人工处理动作（本地上传与云端直存共用）。
const (
	ConflictActionReuse     = "reuse"
	ConflictActionOverwrite = "overwrite"
	ConflictActionRename    = "rename"
	ConflictActionSkip      = "skip"
)

// ResolveConflict 对单个逐文件冲突执行人工选择，然后重新汇总对象状态。
func (s *Service) ResolveConflict(ctx context.Context, taskID, objectURL, rel, action string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case ConflictActionReuse, ConflictActionOverwrite, ConflictActionRename, ConflictActionSkip:
	default:
		return fmt.Errorf("未知动作: %s（reuse|overwrite|rename|skip）", action)
	}
	task, ok := s.taskByID(taskID)
	if !ok {
		return fmt.Errorf("任务不存在: %s", taskID)
	}
	cfg, err := s.taskConfig(task)
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return fmt.Errorf("任务未启用 remote_save")
	}
	st := task.Storage()
	if st == nil {
		return fmt.Errorf("任务无存储")
	}
	obj, err := st.Get(objectURL)
	if err != nil || obj == nil {
		return fmt.Errorf("对象不存在: %s", objectURL)
	}
	status := obj.GetStatus()
	if status != model.StatusCompleted {
		// 云端目标已存在时对象会被置为 failed/failed_permanent；这类冲突仍必须可人工处理。
		if !obj.IsCloudDownload() || (status != model.StatusFailed && status != model.StatusFailedPermanent) {
			return fmt.Errorf("只允许处理 completed 对象（云端冲突允许 failed/failed_permanent）")
		}
	}
	unlock := s.lockObject(obj.URL)
	defer unlock()

	man := s.manifest(taskID)
	entry, ok := man.Get(rel)
	if !ok || entry.Status != model.RemoteFileStatusConflict {
		return fmt.Errorf("文件不是冲突状态: %s", rel)
	}
	if entry.ObjectURL != "" && entry.ObjectURL != objectURL {
		return fmt.Errorf("冲突条目属于其它对象: %s", entry.ObjectURL)
	}
	if obj.IsCloudDownload() {
		return s.resolveCloudConflict(ctx, task, cfg, obj, man, entry, action)
	}

	fs, ferr := s.collectObjectFileSet(task, obj)
	if ferr != nil {
		return ferr
	}
	local := filepath.Join(saveDir(task), filepath.FromSlash(rel))
	if _, lerr := LocalRel(saveDir(task), local); lerr != nil {
		return lerr
	}
	fi, localErr := statRegular(local)
	hasLocal := localErr == nil
	if (action == ConflictActionOverwrite || action == ConflictActionRename) && !hasLocal {
		return localErr
	}
	sha := ""
	var mtime time.Time
	var size int64
	if hasLocal {
		sha, err = hashFile(ctx, local)
		if err != nil {
			return err
		}
		mtime, size = fi.ModTime(), fi.Size()
	} else {
		sha = entry.SHA256
		mtime = time.Unix(0, entry.MTimeNs)
		size = entry.Size
	}
	client, err := s.opts.ClientFor(s.sproxyCfg(), cfg.Volume)
	if err != nil {
		return err
	}
	remote := entry.RemotePath
	if remote == "" {
		remote, err = cfg.RemotePath(rel)
		if err != nil {
			return err
		}
	}
	switch action {
	case ConflictActionSkip:
		entry.Status, entry.Reason, entry.SHA256 = model.RemoteFileStatusSkipped, "user skip", sha
	case ConflictActionReuse:
		rfi, serr := client.Stat(ctx, remote)
		if serr != nil || rfi == nil {
			return fmt.Errorf("reuse: 远端文件不可读: %w", serr)
		}
		if rfi.Checksum == "" {
			return fmt.Errorf("reuse: 远端 sha256 不可得")
		}
		meta, merr := remoteFileMeta(ctx, client, task, obj, rel, remote, cfg)
		if merr != nil {
			return fmt.Errorf("reuse: 构造远端 meta 失败: %w", merr)
		}
		if uerr := client.UpdateMeta(ctx, remote, meta); uerr != nil {
			return fmt.Errorf("reuse: 更新 meta 失败: %w", uerr)
		}
		metaHash, _ := MetaHash(meta)
		entry.Status, entry.Reason, entry.RemoteSHA256, entry.MetaHash = model.RemoteFileStatusReused, "user reuse", rfi.Checksum, metaHash
	case ConflictActionOverwrite:
		if derr := deleteRemote(ctx, client, remote); derr != nil {
			return fmt.Errorf("overwrite: 删除既有远端失败: %w", derr)
		}
		meta, merr := BuildMeta(task, obj, rel, remote, size, mtime, sha, cfg)
		if merr != nil {
			return merr
		}
		sum, uerr := client.Upload(ctx, local, remote)
		if uerr != nil {
			return fmt.Errorf("overwrite: 上传失败: %w", uerr)
		}
		if !strings.EqualFold(sum, sha) {
			return fmt.Errorf("overwrite: 上传后 sha256 不一致")
		}
		if uerr := client.UpdateMeta(ctx, remote, meta); uerr != nil {
			return fmt.Errorf("overwrite: 更新 meta 失败: %w", uerr)
		}
		metaHash, _ := MetaHash(meta)
		entry.Status, entry.Reason, entry.SHA256, entry.RemoteSHA256, entry.MetaHash = model.RemoteFileStatusUploaded, "user overwrite", sha, sha, metaHash
	case ConflictActionRename:
		newRemote := path.Join(path.Dir(remote), withHashSuffix(path.Base(remote), rel))
		meta, merr := BuildMeta(task, obj, rel, newRemote, size, mtime, sha, cfg)
		if merr != nil {
			return merr
		}
		sum, uerr := uploadOrReuse(ctx, client, local, newRemote, sha)
		if uerr != nil {
			return fmt.Errorf("rename: 上传/复用失败: %w", uerr)
		}
		if !strings.EqualFold(sum, sha) {
			return fmt.Errorf("rename: 上传后 sha256 不一致")
		}
		if uerr := client.UpdateMeta(ctx, newRemote, meta); uerr != nil {
			return fmt.Errorf("rename: 更新 meta 失败: %w", uerr)
		}
		metaHash, _ := MetaHash(meta)
		entry.RemotePath = newRemote
		entry.Status, entry.Reason, entry.SHA256, entry.RemoteSHA256, entry.MetaHash = model.RemoteFileStatusUploaded, "user rename", sha, sha, metaHash
	}
	if hasLocal {
		entry.Size, entry.MTimeNs = size, mtime.UnixNano()
	}
	entry.UpdatedAt = time.Now().UTC()
	man.Put(entry)
	if err := man.Flush(); err != nil {
		return err
	}
	// 关键：不要 force 全对象重同步。用户选择只影响该文件；重新校验会把 skip/reuse/rename
	// 立刻改回 conflict（远端内容与本地不同的场景尤为明显）。
	s.refreshObjectState(task, cfg, obj, man, fs, 0)
	return nil
}

func (s *Service) resolveCloudConflict(ctx context.Context, task core.Task, cfg TaskConfig, obj *model.DownloadObject, man *Manifest, entry ManifestEntry, action string) error {
	remote := entry.RemotePath
	if remote == "" {
		return fmt.Errorf("云端冲突缺少远端路径: %s", entry.Path)
	}
	client, err := s.opts.ClientFor(s.sproxyCfg(), cfg.Volume)
	if err != nil {
		return err
	}
	local := filepath.Join(saveDir(task), filepath.FromSlash(entry.Path))
	if _, lerr := LocalRel(saveDir(task), local); lerr != nil {
		return lerr
	}
	localInfo, localErr := statRegular(local)
	hasLocal := localErr == nil
	fs, ferr := s.collectObjectFileSet(task, obj)
	if ferr != nil {
		fs = objectFileSet{byRel: map[string]localFile{}}
	}
	switch action {
	case ConflictActionSkip:
		entry.Status, entry.Reason = model.RemoteFileStatusSkipped, "user skip"
		man.Put(entry)
		_ = man.Flush()
		s.refreshObjectState(task, cfg, obj, man, fs, 0)
		return nil
	case ConflictActionReuse:
		if err := s.recordCloudRemote(ctx, task, cfg, obj, cfg.Volume, entry.Path, remote, true); err != nil {
			return fmt.Errorf("reuse: %w", err)
		}
		_ = man.Flush()
		s.refreshObjectState(task, cfg, obj, man, fs, 0)
		return nil
	case ConflictActionOverwrite:
		if hasLocal {
			sha, herr := hashFile(ctx, local)
			if herr != nil {
				return herr
			}
			meta, merr := BuildMeta(task, obj, entry.Path, remote, localInfo.Size(), localInfo.ModTime(), sha, cfg)
			if merr != nil {
				return merr
			}
			if derr := deleteRemote(ctx, client, remote); derr != nil {
				return fmt.Errorf("overwrite: 删除既有远端失败: %w", derr)
			}
			sum, uerr := client.Upload(ctx, local, remote)
			if uerr != nil {
				return fmt.Errorf("overwrite: 上传失败: %w", uerr)
			}
			if !strings.EqualFold(sum, sha) {
				return fmt.Errorf("overwrite: 上传后 sha256 不一致")
			}
			if uerr := client.UpdateMeta(ctx, remote, meta); uerr != nil {
				return fmt.Errorf("overwrite: 更新 meta 失败: %w", uerr)
			}
			metaHash, _ := MetaHash(meta)
			entry.Status, entry.Reason, entry.SHA256, entry.RemoteSHA256, entry.MetaHash = model.RemoteFileStatusUploaded, "user overwrite", sha, sha, metaHash
			entry.Size, entry.MTimeNs, entry.UpdatedAt = localInfo.Size(), localInfo.ModTime().UnixNano(), time.Now().UTC()
			man.Put(entry)
			_ = man.Flush()
			s.refreshObjectState(task, cfg, obj, man, fs, 0)
			return nil
		}
		// cloud-only：删除远端后重新提交云端下载（显式破坏性动作）。
		if derr := deleteRemote(ctx, client, remote); derr != nil {
			return fmt.Errorf("overwrite: 删除远端失败: %w", derr)
		}
		return s.requeueCloudObject(task, obj, man, entry.Path)
	case ConflictActionRename:
		newRemote := path.Join(path.Dir(remote), withHashSuffix(path.Base(remote), entry.Path))
		if hasLocal {
			sha, herr := hashFile(ctx, local)
			if herr != nil {
				return herr
			}
			meta, merr := BuildMeta(task, obj, entry.Path, newRemote, localInfo.Size(), localInfo.ModTime(), sha, cfg)
			if merr != nil {
				return merr
			}
			sum, uerr := uploadOrReuse(ctx, client, local, newRemote, sha)
			if uerr != nil {
				return fmt.Errorf("rename: 上传/复用失败: %w", uerr)
			}
			if !strings.EqualFold(sum, sha) {
				return fmt.Errorf("rename: 上传后 sha256 不一致")
			}
			if uerr := client.UpdateMeta(ctx, newRemote, meta); uerr != nil {
				return fmt.Errorf("rename: 更新 meta 失败: %w", uerr)
			}
			metaHash, _ := MetaHash(meta)
			entry.RemotePath, entry.Status, entry.Reason = newRemote, model.RemoteFileStatusUploaded, "user rename"
			entry.SHA256, entry.RemoteSHA256, entry.MetaHash = sha, sha, metaHash
			entry.Size, entry.MTimeNs, entry.UpdatedAt = localInfo.Size(), localInfo.ModTime().UnixNano(), time.Now().UTC()
			man.Put(entry)
			_ = man.Flush()
			s.refreshObjectState(task, cfg, obj, man, fs, 0)
			return nil
		}
		// cloud-only：把现有远端改名保留（可能已在上次中断中完成），随后重新下载到原目标。
		if rerr := renameRemoteOrReuse(ctx, client, remote, newRemote, entry.RemoteSHA256); rerr != nil {
			return fmt.Errorf("rename: 远端改名失败: %w", rerr)
		}
		return s.requeueCloudObject(task, obj, man, entry.Path)
	default:
		return fmt.Errorf("未知动作: %s（reuse|overwrite|rename|skip）", action)
	}
}

func (s *Service) requeueCloudObject(task core.Task, obj *model.DownloadObject, man *Manifest, rel string) error {
	clearCloudExtras(obj)
	man.Delete(rel)
	_ = man.Flush()
	obj.SetRemoteSave(nil)
	obj.SetStatus(model.StatusPending)
	if s.opts.Persist != nil {
		s.opts.Persist(task, obj)
	}
	if s.opts.Requeue == nil {
		return fmt.Errorf("无法重新入队云端下载：Requeue 未装配")
	}
	s.opts.Requeue(task, obj)
	return nil
}

func remoteFileMeta(ctx context.Context, client Client, task core.Task, obj *model.DownloadObject, rel, remote string, cfg TaskConfig) (map[string]any, error) {
	fi, err := client.Stat(ctx, remote)
	if err != nil || fi == nil {
		return nil, err
	}
	var mtime time.Time
	if mi, merr := client.GetMeta(ctx, remote); merr == nil && mi.MTime != "" {
		if t, terr := time.Parse(time.RFC3339Nano, mi.MTime); terr == nil {
			mtime = t
		}
	}
	return BuildMeta(task, obj, rel, remote, fi.Size, mtime, fi.Checksum, cfg)
}

func clearCloudExtras(obj *model.DownloadObject) {
	if obj == nil {
		return
	}
	obj.Lock()
	defer obj.Unlock()
	if obj.Extra == nil {
		return
	}
	delete(obj.Extra, "cloud_task_id")
	delete(obj.Extra, "transfer_url")
	delete(obj.Extra, "cloud_task_filename")
	delete(obj.Extra, "transfer_path")
	delete(obj.Extra, "transfer_volume")
}

func statRegular(p string) (os.FileInfo, error) {
	st, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("不是常规文件: %s", p)
	}
	return st, nil
}
