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

// ErrRemoteChecksum 表示云端内容无法与本地/远端记录用 sha256 证明一致，应进入人工冲突流程。
var ErrRemoteChecksum = errors.New("remote-save: 云端 checksum 不一致")

// PrepareCloud 在提交 sproxy 云端下载前，把转存目标路径改写为
// <dir>/<objectKey>/<filename>（object_dirs=false 时保持 <dir>/<rel>）。
// 目标已存在时 fail-closed（记录 conflict），避免服务端下载后才发现 ErrExists。
func (s *Service) PrepareCloud(task core.Task, obj *model.DownloadObject) error {
	if s == nil || task == nil || obj == nil || !obj.IsCloudDownload() {
		return nil
	}
	unlock := s.lockObject(obj.URL)
	defer unlock()
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
		used := map[string]string{}
		for _, m := range files {
			origPath := strings.TrimSpace(asString(m["path"]))
			name := path.Base(origPath)
			if name == "." || name == "" || name == "/" {
				name = path.Base(strings.TrimSpace(asString(m["target"])))
			}
			if name == "." || name == "" || name == "/" {
				name = sanitizeRemoteSegment(path.Base(filepathToSlash(obj.SavePath)), false)
			}
			base := sanitizeRemoteSegment(name, false)
			target := cloudPathForFile(cfg, key, origPath, base)
			if prev, ok := used[strings.ToLower(target)]; ok && prev != origPath {
				base = withHashSuffix(base, origPath)
				target = cloudPathForFile(cfg, key, origPath, base)
			}
			used[strings.ToLower(target)] = origPath
			m["path"] = target
			targets = append(targets, target)
		}
		obj.Extra["files"] = files
	}
	obj.Unlock()

	client, cerr := s.opts.ClientFor(s.sproxyCfg(), cfg.Volume)
	if cerr != nil {
		return fmt.Errorf("cloud target check: %w", cerr)
	}
	statCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var conflicts []string
	for _, target := range targets {
		fi, serr := client.Stat(statCtx, target)
		if serr == nil && fi != nil {
			conflicts = append(conflicts, target)
			continue
		}
		if serr != nil && !errors.Is(serr, sproxyclient.ErrNotFound) {
			// 无法确认目标状态时 fail-closed，避免误覆盖。
			return fmt.Errorf("cloud target stat: %w", serr)
		}
	}
	if len(conflicts) > 0 {
		state := obj.GetRemoteSave()
		if state == nil {
			state = &model.RemoteSaveState{}
		}
		state.Status, state.Volume, state.Dir = model.RemoteSaveStatusConflict, cfg.Volume, cfg.Dir
		state.FilesTotal, state.FilesDone, state.BytesTotal, state.BytesDone = len(conflicts), 0, 0, 0
		state.FilesTruncated, state.RemotePath = false, ""
		state.PlayURL, state.PlayExpiresAt = "", time.Time{}
		state.Conflicts, state.LastError, state.UpdatedAt = len(conflicts), "cloud target exists: "+strings.Join(conflicts, ", "), time.Now().UTC()
		state.Files = nil
		man := s.manifest(task.ID())
		for _, target := range conflicts {
			fi, _ := client.Stat(statCtx, target)
			size, checksum := int64(0), ""
			if fi != nil {
				size, checksum = fi.Size, fi.Checksum
			}
			rel := remoteRelToTarget(cfg, target)
			man.Put(ManifestEntry{
				Path: rel, ObjectURL: obj.URL, RemotePath: target, Size: size,
				RemoteSHA256: checksum, Status: model.RemoteFileStatusConflict, Reason: "target_exists", Origin: "cloud",
			})
			state.Files = append(state.Files, model.RemoteFileState{
				Path: rel, RemotePath: target, Size: size,
				RemoteSHA256: checksum, Status: model.RemoteFileStatusConflict, Reason: "target_exists",
			})
		}
		_ = man.Flush()
		obj.SetRemoteSave(state)
		if s.opts.Persist != nil {
			s.opts.Persist(task, obj)
		}
		return fmt.Errorf("%w: %s", ErrTargetExists, strings.Join(conflicts, ", "))
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
	unlock := s.lockObject(obj.URL)
	defer unlock()
	transferURL := extraString(obj, "transfer_url")
	if transferURL == "" {
		s.persistState(task, obj, cfg, &model.RemoteSaveState{Status: model.RemoteSaveStatusFailed, Volume: cfg.Volume, Dir: cfg.Dir, LastError: "云端完成但缺少 transfer_url"})
		return
	}
	volume, primary, err := parseTransferURL(transferURL)
	if err != nil {
		s.Logger().Warn("remote-save cloud transfer_url 解析失败", "task_id", task.ID(), "url", transferURL, "error", err)
		return
	}
	targets := []string{primary}
	if files := normalizeCloudFiles(obj.Extra["files"]); len(files) > 0 {
		for _, m := range files {
			if p := strings.TrimSpace(asString(m["path"])); p != "" {
				targets = append(targets, p)
			}
		}
	}
	seen := map[string]bool{}
	var firstErr error
	man := s.manifest(task.ID())
	for _, remote := range targets {
		if remote == "" || seen[remote] {
			continue
		}
		seen[remote] = true
		rel := remoteRelToTarget(cfg, remote)
		if obj.SavePath != "" && remote == primary {
			if lrel, lerr := LocalRel(saveDir(task), obj.SavePath); lerr == nil {
				rel = lrel
			}
		}
		if e, ok := man.GetFor(obj.URL, rel); ok && e.Status == model.RemoteFileStatusSkipped {
			continue
		}
		// 云端目标冲突的 skip 可能按 target rel 记录；本地 rel 不同时也要尊重。
		if targetRel := remoteRelToTarget(cfg, remote); targetRel != rel {
			if e, ok := man.GetFor(obj.URL, targetRel); ok && e.Status == model.RemoteFileStatusSkipped {
				continue
			}
		}
		if rerr := s.recordCloudRemote(ctx, task, cfg, obj, volume, rel, remote, false); rerr != nil {
			s.Logger().Warn("remote-save cloud record failed", "task_id", task.ID(), "remote", remote, "error", rerr)
			st := model.RemoteFileStatusFailed
			if errors.Is(rerr, ErrRemoteChecksum) {
				st = model.RemoteFileStatusConflict
			}
			man.Put(ManifestEntry{Path: rel, ObjectURL: obj.URL, RemotePath: remote, Status: st, Origin: "cloud", Reason: rerr.Error()})
			if firstErr == nil {
				firstErr = rerr
			}
		}
	}
	setTransferURL(obj, volume, primary)
	_ = man.Flush()
	fs, ferr := s.collectObjectFileSet(task, obj)
	if ferr != nil {
		fs = objectFileSet{byRel: map[string]localFile{}}
	}
	delta := 0
	if firstErr != nil {
		delta = 1
	}
	s.refreshObjectState(task, cfg, obj, man, fs, delta)
}

// recordCloudRemote 回读 sproxy 远端坐标的 sha256/mtime，写 manifest 与 transfer_url。
// acceptRemote=true 表示用户明确接受远端内容（reuse），不再要求本地 sha 相同；
// 否则本地文件存在时必须与远端 sha 一致。
func (s *Service) recordCloudRemote(ctx context.Context, task core.Task, cfg TaskConfig, obj *model.DownloadObject, volume, rel, remote string, acceptRemote bool) error {
	if volume == "" {
		volume = cfg.Volume
	}
	client, err := s.opts.ClientFor(s.sproxyCfg(), volume)
	if err != nil {
		return err
	}
	fi, err := client.Stat(ctx, remote)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	if fi == nil || fi.Checksum == "" {
		return fmt.Errorf("%w: 远端缺少 sha256: %s", ErrRemoteChecksum, remote)
	}
	if rel == "" {
		rel = remoteRelToTarget(cfg, remote)
	}
	size := fi.Size
	mtime := time.Time{}
	if mi, merr := client.GetMeta(ctx, remote); merr == nil && mi.MTime != "" {
		if t, terr := time.Parse(time.RFC3339Nano, mi.MTime); terr == nil {
			mtime = t
		}
	}
	localSHA := fi.Checksum
	localRel := ""
	if obj.SavePath != "" {
		if rel, rerr := LocalRel(saveDir(task), obj.SavePath); rerr == nil {
			localRel = rel
		}
	}
	if localRel == rel {
		if st, serr := os.Stat(obj.SavePath); serr == nil && st.Mode().IsRegular() {
			sum, herr := hashFile(ctx, obj.SavePath)
			if herr != nil {
				return herr
			}
			if !acceptRemote && !strings.EqualFold(sum, fi.Checksum) {
				return fmt.Errorf("%w: 本地文件与远端 sha256 不一致", ErrRemoteChecksum)
			}
			localSHA = sum
			size = st.Size()
			mtime = st.ModTime()
		}
	}
	meta, merr := BuildMeta(task, obj, rel, remote, size, mtime, fi.Checksum, cfg)
	if merr != nil {
		return merr
	}
	if uerr := client.UpdateMeta(ctx, remote, meta); uerr != nil {
		return fmt.Errorf("补录 meta: %w", uerr)
	}
	metaHash, _ := MetaHash(meta)
	man := s.manifest(task.ID())
	man.Put(ManifestEntry{
		Path: rel, ObjectURL: obj.URL, RemotePath: remote, Size: size, MTimeNs: mtime.UnixNano(),
		SHA256: localSHA, RemoteSHA256: fi.Checksum, MetaHash: metaHash,
		Origin: "cloud", Status: model.RemoteFileStatusUploaded,
	})
	setTransferURL(obj, volume, remote)
	return nil
}

func setTransferURL(obj *model.DownloadObject, volume, remote string) {
	obj.Lock()
	defer obj.Unlock()
	if obj.Extra == nil {
		obj.Extra = map[string]any{}
	}
	obj.Extra["transfer_url"] = "local://" + volume + "/" + remote
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

// cloudRemotePaths 返回云端对象当前有效的转存目标（transfer_url + files[] 路径）。
func cloudRemotePaths(obj *model.DownloadObject) []string {
	if obj == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	if u := extraString(obj, "transfer_url"); u != "" {
		if _, rel, err := parseTransferURL(u); err == nil {
			add(rel)
		}
	}
	obj.RLock()
	rawFiles := obj.Extra["files"]
	obj.RUnlock()
	for _, m := range normalizeCloudFiles(rawFiles) {
		add(asString(m["path"]))
	}
	return out
}
