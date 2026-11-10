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

	"github.com/cocomhub/download-manager/model"
)

// 冲突人工处理动作（B 阶段 UI；A 阶段仅提供后端接口）。
const (
	ConflictActionReuse     = "reuse"
	ConflictActionOverwrite = "overwrite"
	ConflictActionRename    = "rename"
	ConflictActionSkip      = "skip"
)

// ResolveConflict 对单个逐文件冲突执行人工选择，然后重新汇总对象状态。
func (s *Service) ResolveConflict(ctx context.Context, taskID, objectURL, rel, action string) error {
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
	if obj.GetStatus() != model.StatusCompleted {
		return fmt.Errorf("只允许处理 completed 对象")
	}
	man := s.manifest(taskID)
	entry, ok := man.Get(rel)
	if !ok || entry.Status != model.RemoteFileStatusConflict {
		return fmt.Errorf("文件不是冲突状态: %s", rel)
	}
	client, err := s.opts.ClientFor(s.sproxyCfg(), cfg.Volume)
	if err != nil {
		return err
	}
	local := filepath.Join(saveDir(task), filepath.FromSlash(rel))
	fi, err := statRegular(local)
	if err != nil {
		return err
	}
	sha, err := hashFile(ctx, local)
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
	meta, err := BuildMeta(task, obj, rel, remote, fi.Size(), fi.ModTime(), sha, cfg)
	if err != nil {
		return err
	}
	metaHash, _ := MetaHash(meta)
	switch strings.ToLower(strings.TrimSpace(action)) {
	case ConflictActionSkip:
		entry.Status, entry.Reason, entry.SHA256 = model.RemoteFileStatusSkipped, "user skip", sha
	case ConflictActionReuse:
		if rfi, serr := client.Stat(ctx, remote); serr != nil || rfi == nil {
			return fmt.Errorf("reuse: 远端文件不可读: %w", serr)
		} else {
			entry.RemoteSHA256 = rfi.Checksum
		}
		// 接受远端内容；meta 仍按 mongo 对象补录，便于查看。
		if uerr := client.UpdateMeta(ctx, remote, meta); uerr != nil {
			return fmt.Errorf("reuse: 更新 meta 失败: %w", uerr)
		}
		entry.Status, entry.Reason, entry.SHA256, entry.MetaHash = model.RemoteFileStatusReused, "user reuse", sha, metaHash
	case ConflictActionOverwrite:
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
		entry.Status, entry.Reason, entry.SHA256, entry.RemoteSHA256, entry.MetaHash = model.RemoteFileStatusUploaded, "user overwrite", sha, sha, metaHash
	case ConflictActionRename:
		newRemote := path.Join(path.Dir(remote), withHashSuffix(path.Base(remote), rel))
		meta, err = BuildMeta(task, obj, rel, newRemote, fi.Size(), fi.ModTime(), sha, cfg)
		if err != nil {
			return err
		}
		metaHash, _ = MetaHash(meta)
		sum, uerr := client.Upload(ctx, local, newRemote)
		if uerr != nil {
			return fmt.Errorf("rename: 上传失败: %w", uerr)
		}
		if !strings.EqualFold(sum, sha) {
			return fmt.Errorf("rename: 上传后 sha256 不一致")
		}
		if uerr := client.UpdateMeta(ctx, newRemote, meta); uerr != nil {
			return fmt.Errorf("rename: 更新 meta 失败: %w", uerr)
		}
		entry.RemotePath = newRemote
		entry.Status, entry.Reason, entry.SHA256, entry.RemoteSHA256, entry.MetaHash = model.RemoteFileStatusUploaded, "user rename", sha, sha, metaHash
	default:
		return fmt.Errorf("未知动作: %s（reuse|overwrite|rename|skip）", action)
	}
	entry.Size, entry.MTimeNs, entry.UpdatedAt = fi.Size(), fi.ModTime().UnixNano(), time.Now().UTC()
	man.Put(entry)
	if err := man.Flush(); err != nil {
		return err
	}
	s.syncObject(task, cfg, obj, false)
	return nil
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
