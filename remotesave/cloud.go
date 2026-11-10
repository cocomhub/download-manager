// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	sproxyclient "github.com/cocomhub/sproxy/pkg/client"
	"github.com/cocomhub/sproxy/pkg/sproxysig"
)

// ErrTargetExists 表示云端转存目标已存在（必须先人工选择 reuse/overwrite/rename）。
var ErrTargetExists = errors.New("remote-save: 目标已存在")

// PrepareCloud 在提交 sproxy 云端下载前，把转存目标路径改写为
// <dir>/<objectKey>/<filename>（object_dirs=false 时保持 <dir>/<rel>）。
// 目标已存在时 fail-closed（记录 conflict），避免服务端下载后才发现 ErrExists。
func (s *Service) PrepareCloud(task core.Task, obj *model.DownloadObject) error {
	if s == nil || task == nil || obj == nil || !obj.IsCloudDownload() {
		return nil
	}
	cfg, err := s.taskConfig(task)
	if err != nil || !cfg.Enabled {
		return err
	}
	key := ObjectKey(obj)
	obj.Lock()
	if obj.Extra == nil {
		obj.Extra = map[string]any{}
	}
	obj.Extra["transfer_volume"] = cfg.Volume
	files := normalizeCloudFiles(obj.Extra["files"])
	var targets []string
	if len(files) == 0 {
		name := "video.mp4"
		if b := sanitizeRemoteSegment(path.Base(filepathToSlash(obj.SavePath)), false); b != "" && b != "." {
			name = b
		}
		target := cloudPath(cfg, key, name)
		obj.Extra["transfer_path"] = target
		targets = append(targets, target)
	} else {
		for _, m := range files {
			origPath := strings.TrimSpace(asString(m["path"]))
			name := path.Base(origPath)
			if name == "." || name == "" || name == "/" {
				name = path.Base(strings.TrimSpace(asString(m["target"])))
			}
			if name == "." || name == "" || name == "/" {
				name = sanitizeRemoteSegment(path.Base(filepathToSlash(obj.SavePath)), false)
			}
			target := cloudPathForFile(cfg, key, origPath, sanitizeRemoteSegment(name, false))
			m["path"] = target
			targets = append(targets, target)
		}
		obj.Extra["files"] = files
	}
	obj.Unlock()

	if client, cerr := s.opts.ClientFor(s.sproxyCfg(), cfg.Volume); cerr == nil {
		statCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		for _, target := range targets {
			fi, serr := client.Stat(statCtx, target)
			if serr == nil && fi != nil {
				state := obj.GetRemoteSave()
				if state == nil {
					state = &model.RemoteSaveState{}
				}
				state.Status, state.Volume, state.Dir = model.RemoteSaveStatusConflict, cfg.Volume, cfg.Dir
				state.Conflicts, state.LastError, state.UpdatedAt = 1, "cloud target exists: "+target, time.Now().UTC()
				state.Files = append(state.Files, model.RemoteFileState{
					Path: path.Base(target), RemotePath: target, Size: fi.Size,
					RemoteSHA256: fi.Checksum, Status: model.RemoteFileStatusConflict, Reason: "target_exists",
				})
				obj.SetRemoteSave(state)
				if s.opts.Persist != nil {
					s.opts.Persist(task, obj)
				}
				return fmt.Errorf("%w: %s", ErrTargetExists, target)
			}
			if serr != nil && !errors.Is(serr, sproxyclient.ErrNotFound) {
				// 无法确认目标状态时 fail-closed，避免误覆盖。
				return fmt.Errorf("cloud target stat: %w", serr)
			}
		}
	}
	return nil
}

func cloudPath(cfg TaskConfig, key, name string) string {
	if cfg.ObjectDirs {
		return path.Join(cfg.Dir, key, name)
	}
	return path.Join(cfg.Dir, name)
}

func cloudPathForFile(cfg TaskConfig, key, origPath, base string) string {
	if cfg.ObjectDirs {
		return path.Join(cfg.Dir, key, base)
	}
	if safe, err := sanitizeRemoteRel(origPath); err == nil {
		return path.Join(cfg.Dir, safe)
	}
	return path.Join(cfg.Dir, base)
}

// NotifyCloudCompleted 在云端下载/转存成功后记录远端坐标、sha256 与播放 URL。
func (s *Service) NotifyCloudCompleted(task core.Task, obj *model.DownloadObject) {
	if s == nil || task == nil || obj == nil || !obj.IsCloudDownload() {
		return
	}
	s.enqueue(request{taskID: task.ID(), url: obj.URL, cloud: true})
}

func (s *Service) syncCloud(ctx context.Context, task core.Task, cfg TaskConfig, obj *model.DownloadObject) {
	transferURL := extraString(obj, "transfer_url")
	if transferURL == "" {
		return
	}
	volume, remote, err := parseTransferURL(transferURL)
	if err != nil {
		s.Logger().Warn("remote-save cloud transfer_url 解析失败", "task_id", task.ID(), "url", transferURL, "error", err)
		return
	}
	if volume == "" {
		volume = cfg.Volume
	}
	client, err := s.opts.ClientFor(s.sproxyCfg(), volume)
	if err != nil {
		s.Logger().Error("remote-save cloud client init failed", "task_id", task.ID(), "error", err)
		return
	}
	fi, err := client.Stat(ctx, remote)
	if err != nil || fi == nil {
		s.Logger().Warn("remote-save cloud stat failed", "task_id", task.ID(), "remote", remote, "error", err)
		return
	}
	if fi.Checksum == "" {
		s.Logger().Warn("remote-save cloud 缺少远端 sha256，拒绝标记同步", "task_id", task.ID(), "remote", remote)
		return
	}
	localRel := path.Base(remote)
	size := fi.Size
	var mtime time.Time
	if mi, merr := client.GetMeta(ctx, remote); merr == nil && mi.MTime != "" {
		if t, terr := time.Parse(time.RFC3339Nano, mi.MTime); terr == nil {
			mtime = t
		}
	}
	if st, serr := os.Stat(obj.SavePath); serr == nil && st.Mode().IsRegular() {
		localRel = filepathToSlash(obj.SavePath)
		if rel, rerr := LocalRel(saveDir(task), obj.SavePath); rerr == nil {
			localRel = rel
		}
		sum, herr := hashFile(ctx, obj.SavePath)
		if herr != nil || !strings.EqualFold(sum, fi.Checksum) {
			s.Logger().Warn("remote-save cloud 本地文件与远端 sha256 不一致", "task_id", task.ID(), "path", obj.SavePath)
			return
		}
		size, mtime = st.Size(), st.ModTime()
	}
	meta, merr := BuildMeta(task, obj, localRel, remote, size, mtime, fi.Checksum, cfg)
	if merr == nil {
		if uerr := client.UpdateMeta(ctx, remote, meta); uerr != nil {
			s.Logger().Warn("remote-save cloud 补录 meta 失败", "task_id", task.ID(), "error", uerr)
		}
	}
	metaHash, _ := MetaHash(meta)
	man := s.manifest(task.ID())
	man.Put(ManifestEntry{
		Path: localRel, RemotePath: remote, Size: size, MTimeNs: mtime.UnixNano(),
		SHA256: fi.Checksum, RemoteSHA256: fi.Checksum, MetaHash: metaHash,
		Origin: "cloud", Status: model.RemoteFileStatusUploaded,
	})
	_ = man.Flush()
	state := &model.RemoteSaveState{
		Status: model.RemoteSaveStatusSynced, Volume: volume, Dir: cfg.Dir,
		FilesTotal: 1, FilesDone: 1, BytesTotal: size, BytesDone: size,
		RemotePath: remote, UpdatedAt: time.Now().UTC(),
	}
	state.Files, _ = ToModelFiles(man.Snapshot(), []string{localRel}, s.maxFiles())
	if play, exp, perr := s.buildPlayURL(state); perr == nil {
		state.PlayURL, state.PlayExpiresAt = play, exp
	}
	s.persistState(task, obj, cfg, state)
}

// PlayURL 生成/刷新签名播放 URL，并写回对象状态。
func (s *Service) PlayURL(task core.Task, obj *model.DownloadObject) (string, time.Time, error) {
	if task == nil || obj == nil {
		return "", time.Time{}, fmt.Errorf("nil task/object")
	}
	state := obj.GetRemoteSave()
	if state == nil || state.RemotePath == "" {
		return "", time.Time{}, fmt.Errorf("对象尚未完成远程保存")
	}
	play, exp, err := s.buildPlayURL(state)
	if err != nil {
		return "", time.Time{}, err
	}
	obj.UpdateRemoteSave(func(st *model.RemoteSaveState) bool {
		st.PlayURL, st.PlayExpiresAt = play, exp
		return true
	})
	if s.opts.Persist != nil {
		s.opts.Persist(task, obj)
	}
	return play, exp, nil
}

func (s *Service) buildPlayURL(state *model.RemoteSaveState) (string, time.Time, error) {
	sp := s.sproxyCfg()
	if sp.PlayBaseURL == "" {
		return "", time.Time{}, fmt.Errorf("sproxy.play_base_url 未配置")
	}
	if sp.AccessKey == "" || sp.AccessKeySecret == "" || sp.AccessKeyID == "" {
		return "", time.Time{}, fmt.Errorf("播放链接需要 SproxySig 三件套（access_key/access_key_secret/access_key_id）")
	}
	ttl := sp.PlayTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	now := time.Now()
	exp := now.Add(ttl)
	q := url.Values{}
	q.Set("filename", state.RemotePath)
	if state.Volume != "" {
		q.Set("volume", state.Volume)
	}
	query := q.Encode()
	h := sproxysig.Header{
		Version: sproxysig.Version, AK: sp.AccessKey, EntryID: sp.AccessKeyID,
		TS: now.UnixMilli(), Exp: exp.UnixMilli(), Nonce: sproxysig.NewNonce(),
		BodySHA256: sproxysig.EmptyBodyHash(),
	}
	sig := sproxysig.SignAndFormat(sp.AccessKeySecret, h, "GET", "/download", query)
	full := strings.TrimRight(sp.PlayBaseURL, "/") + "/download?" + query + "&spx_sig=" + url.QueryEscape(sig)
	return full, exp, nil
}

func parseTransferURL(raw string) (volume, rel string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("非法 transfer_url: %s", raw)
	}
	rel = strings.TrimPrefix(u.Path, "/")
	if rel == "" {
		return "", "", fmt.Errorf("transfer_url 缺路径: %s", raw)
	}
	return u.Host, rel, nil
}

func extraString(obj *model.DownloadObject, key string) string {
	if obj == nil {
		return ""
	}
	obj.RLock()
	defer obj.RUnlock()
	s, _ := obj.Extra[key].(string)
	return s
}

func normalizeCloudFiles(raw any) []map[string]any {
	switch list := raw.(type) {
	case []map[string]string:
		out := make([]map[string]any, 0, len(list))
		for _, m := range list {
			cp := map[string]any{}
			for k, v := range m {
				cp[k] = v
			}
			out = append(out, cp)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, 0, len(list))
		for _, m := range list {
			cp := map[string]any{}
			maps.Copy(cp, m)
			out = append(out, cp)
		}
		return out
	case []any:
		out := make([]map[string]any, 0, len(list))
		for _, it := range list {
			if m, ok := it.(map[string]any); ok {
				cp := map[string]any{}
				maps.Copy(cp, m)
				out = append(out, cp)
			}
		}
		return out
	default:
		return nil
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
