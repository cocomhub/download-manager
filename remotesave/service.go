// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	sproxyclient "github.com/cocomhub/sproxy/pkg/client"
)

// Options 是 Service 的装配注入面（测试用 fake client / fake persister）。
type Options struct {
	Cfg          func() *config.Config
	Tasks        func() []core.Task
	WorkDir      string
	ClientFor    func(sp config.Sproxy, volume string) (Client, error)
	Persist      func(core.Task, *model.DownloadObject)
	Requeue      func(core.Task, *model.DownloadObject)
	IsObjectGone func(core.Task, string) bool
	Logger       *slog.Logger
}

// Service 是远程保存引擎：只处理 completed 对象，逐文件 sha256 校验后上传。
type Service struct {
	opts      Options
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	queue     chan request
	inflight  sync.Map
	manifests sync.Map // taskID -> *Manifest
	mu        sync.Mutex
	objMu     sync.Mutex
	objLocks  map[string]*objectLock
	started   bool
}

type objectLock struct {
	mu   sync.Mutex
	refs int
}

type request struct {
	taskID string
	url    string // 空 = 全任务 completed 对象
	force  bool
	cloud  bool // 云端转存完成后的坐标记录
}

// New 创建 Service（不启动）。
func New(opts Options) *Service {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.ClientFor == nil {
		opts.ClientFor = func(sp config.Sproxy, volume string) (Client, error) {
			return NewSproxyClient(sp, volume, false)
		}
	}
	return &Service{opts: opts, queue: make(chan request, 256)}
}

// Start 启动 worker 与周期对账（幂等）。
func (s *Service) Start() {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx, s.cancel = ctx, cancel
	cfg := s.opts.Cfg()
	n := 2
	if cfg != nil && cfg.RemoteSave.Concurrency > 0 {
		n = cfg.RemoteSave.Concurrency
	}
	s.mu.Unlock()

	for i := 0; i < n; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	s.wg.Add(1)
	go s.reconciler()
	s.Logger().Info("remote-save started", "workers", n)
}

// Stop 取消在途工作并有界等待（分块会话服务端保留，重启后可续传）。
func (s *Service) Stop(ctx context.Context) {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	s.started = false
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		s.Logger().Warn("remote-save stop timed out")
	}
	s.flushAllManifests()
}

// Reload 在配置热更新后调用：由 disabled → enabled 时自动启动（worker 数不动态调整）。
func (s *Service) Reload() {
	s.Logger().Info("remote-save config reloaded")
	if s.Enabled() && !s.Started() {
		s.Start()
	}
}

// Enabled 报告是否至少有一个任务启用了 remote_save。
func (s *Service) Enabled() bool {
	cfg := s.opts.Cfg()
	if cfg == nil {
		return false
	}
	for i := range cfg.Tasks {
		tc, err := ParseTaskConfig(cfg.Tasks[i].Extra)
		if err == nil && tc.Enabled {
			return true
		}
	}
	return false
}

// Started 报告服务是否已启动。
func (s *Service) Started() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

// NotifyObjectCompleted 在对象下载完成（含小对象）后入队；非阻塞。
func (s *Service) NotifyObjectCompleted(task core.Task, obj *model.DownloadObject) {
	if s == nil || task == nil || obj == nil {
		return
	}
	s.enqueue(request{taskID: task.ID(), url: obj.URL})
}

// TriggerTask 入队任务的全部 completed 对象；force=true 忽略 manifest 快路径。
func (s *Service) TriggerTask(taskID string, force bool) error {
	if s == nil {
		return fmt.Errorf("remote-save 未启动")
	}
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if !started {
		return fmt.Errorf("remote-save 未启动")
	}
	if _, ok := s.taskByID(taskID); !ok {
		return fmt.Errorf("任务不存在: %s", taskID)
	}
	s.enqueue(request{taskID: taskID, force: force})
	return nil
}

func (s *Service) enqueue(r request) {
	key := r.taskID + "\x00" + r.url
	if _, loaded := s.inflight.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	select {
	case s.queue <- r:
	default:
		s.inflight.Delete(key)
		s.Logger().Warn("remote-save queue full, dropping request", "task_id", r.taskID, "url", r.url)
	}
}

func (s *Service) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case r := <-s.queue:
			s.handle(r)
			s.inflight.Delete(r.taskID + "\x00" + r.url)
		}
	}
}

func (s *Service) reconciler() {
	defer s.wg.Done()
	timer := time.NewTimer(0)
	defer timer.Stop()
	lastDeep := s.loadLastDeep()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
			interval := 10 * time.Minute
			deepEvery := 24 * time.Hour
			if cfg := s.opts.Cfg(); cfg != nil {
				if cfg.RemoteSave.ReconcileInterval > 0 {
					interval = cfg.RemoteSave.ReconcileInterval
				}
				if cfg.RemoteSave.DeepReconcileInterval > 0 {
					deepEvery = cfg.RemoteSave.DeepReconcileInterval
				}
			}
			deep := time.Since(lastDeep) >= deepEvery
			if deep {
				lastDeep = time.Now()
				s.saveLastDeep(lastDeep)
			}
			s.enqueueAll(deep)
			timer.Reset(interval)
		}
	}
}

func (s *Service) enqueueAll(force bool) {
	for _, task := range s.opts.Tasks() {
		cfg, err := s.taskConfig(task)
		if err != nil {
			s.Logger().Error("remote-save config invalid", "task_id", task.ID(), "error", err)
			continue
		}
		if !cfg.Enabled {
			continue
		}
		s.enqueue(request{taskID: task.ID(), force: force})
	}
}

func (s *Service) handle(r request) {
	task, ok := s.taskByID(r.taskID)
	if !ok {
		return
	}
	cfg, err := s.taskConfig(task)
	if err != nil || !cfg.Enabled {
		if err != nil {
			s.Logger().Error("remote-save config invalid", "task_id", task.ID(), "error", err)
		}
		return
	}
	objs, err := s.listCompleted(task, r.url)
	if err != nil {
		s.Logger().Error("remote-save list completed failed", "task_id", task.ID(), "error", err)
		return
	}
	man := s.manifest(task.ID())
	for _, obj := range objs {
		if s.ctx.Err() != nil {
			return
		}
		if obj.IsCloudDownload() {
			s.syncCloud(s.ctx, task, cfg, obj)
			if r.cloud {
				continue
			}
		}
		s.syncObject(task, cfg, obj, r.force)
	}
	if r.url == "" && cfg.MirrorDelete {
		s.mirrorCleanup(s.ctx, task, cfg, objs, man)
	}
}

// mirrorCleanup 在任务级对账后删除「已不在任何 completed 对象文件集」且由本工具记录过的远端文件。
// 安全边界：只删 manifest 中 ObjectURL 可判定为已删除/已完成对象已无此文件的条目；
// 对象仍存在但非 completed（临时失败/重下）不删；未知远端文件永不删。
func (s *Service) mirrorCleanup(ctx context.Context, task core.Task, cfg TaskConfig, objs []*model.DownloadObject, man *Manifest) {
	if s.mirrorDeleteUnsafe(task, cfg) {
		s.Logger().Warn("remote-save mirror cleanup skipped: another enabled task shares the same volume/dir", "task_id", task.ID(), "volume", cfg.Volume, "dir", cfg.Dir)
		return
	}
	completed := make(map[string]*model.DownloadObject, len(objs))
	for _, obj := range objs {
		completed[obj.URL] = obj
	}
	expected := map[string]bool{}
	filesKnown := map[string]bool{}
	entries := man.Snapshot()
	for _, obj := range objs {
		fs, err := s.collectObjectFileSet(task, obj)
		if err != nil {
			filesKnown[obj.URL] = false
			continue
		}
		filesKnown[obj.URL] = true
		for _, p := range objectRemotePaths(obj, fs.files, fs.rels, cfg) {
			expected[p] = true
		}
		if obj.IsCloudDownload() {
			paths := cloudRemotePaths(obj)
			if len(paths) == 0 {
				filesKnown[obj.URL] = false
			}
			for _, p := range paths {
				expected[p] = true
			}
		}
		for _, e := range entries {
			if e.ObjectURL == obj.URL && strings.HasPrefix(e.Reason, "user rename") && e.RemotePath != "" {
				expected[e.RemotePath] = true
			}
		}
	}
	client, err := s.opts.ClientFor(s.sproxyCfg(), cfg.Volume)
	if err != nil {
		return
	}
	st := task.Storage()
	changed := map[string]*model.DownloadObject{}
	for rel, e := range entries {
		if expected[e.RemotePath] || e.ObjectURL == "" || e.RemotePath == "" {
			continue
		}
		if cfg.Dir != "" && !strings.HasPrefix(e.RemotePath, strings.TrimSuffix(cfg.Dir, "/")+"/") {
			continue
		}
		// 删除前始终重新确认对象状态，避免对账列表读取后对象已变成 failed/pending 时误删。
		if st == nil {
			continue
		}
		obj, gerr := st.Get(e.ObjectURL)
		if gerr == nil && obj != nil && obj.GetStatus() != model.StatusCompleted {
			continue // 对象仍存在但非 completed，镜像清理不介入
		}
		if gerr != nil && !errors.Is(gerr, core.ErrObjectNotFound) {
			s.Logger().Warn("remote-save mirror lookup failed, skip delete", "task_id", task.ID(), "object", e.ObjectURL, "error", gerr)
			continue
		}
		if _, still := completed[e.ObjectURL]; still && !filesKnown[e.ObjectURL] {
			// 对象仍是 completed，但本次无法可靠列举文件：不动，避免临时 IO 错误导致误删。
			continue
		}
		if err := client.Delete(ctx, e.RemotePath); err != nil && !errors.Is(err, sproxyclient.ErrNotFound) {
			s.Logger().Warn("remote-save mirror delete failed", "task_id", task.ID(), "remote", e.RemotePath, "error", err)
			continue
		}
		man.Delete(rel)
		if obj := completed[e.ObjectURL]; obj != nil {
			changed[e.ObjectURL] = obj
		}
		s.Logger().Info("remote-save mirror deleted orphan", "task_id", task.ID(), "remote", e.RemotePath)
	}
	_ = man.Flush()
	for _, obj := range changed {
		fs, ferr := s.collectObjectFileSet(task, obj)
		if ferr == nil {
			s.refreshObjectState(task, cfg, obj, man, fs, 0)
		}
	}
}

func (s *Service) mirrorDeleteUnsafe(task core.Task, cfg TaskConfig) bool {
	for _, other := range s.opts.Tasks() {
		if other == nil || other.ID() == task.ID() {
			continue
		}
		oc, err := s.taskConfig(other)
		if err != nil || !oc.Enabled || oc.Volume != cfg.Volume {
			continue
		}
		if remoteDirsOverlap(oc.Dir, cfg.Dir) {
			return true
		}
	}
	return false
}

func remoteDirsOverlap(a, b string) bool {
	a, b = path.Clean(strings.TrimSpace(a)), path.Clean(strings.TrimSpace(b))
	if a == "." || b == "." {
		return true
	}
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func (s *Service) listCompleted(task core.Task, url string) ([]*model.DownloadObject, error) {
	st := task.Storage()
	if st == nil {
		return nil, fmt.Errorf("任务无存储")
	}
	if url != "" {
		obj, err := st.Get(url)
		if err != nil || obj == nil {
			return nil, err
		}
		if obj.GetStatus() != model.StatusCompleted {
			return nil, nil
		}
		return []*model.DownloadObject{obj}, nil
	}
	const batch = 200
	var out []*model.DownloadObject
	for off := int64(0); ; off += batch {
		objs, err := st.Search(&core.StorageQuery{
			Filter: core.StorageFilter{TaskIDs: []string{task.ID()}, Statuses: []string{model.StatusCompleted}},
			Offset: off,
			Limit:  batch,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
		if int64(len(objs)) < batch || len(objs) == 0 {
			break
		}
	}
	return out, nil
}

func (s *Service) syncObject(task core.Task, cfg TaskConfig, obj *model.DownloadObject, force bool) {
	if obj == nil || obj.GetStatus() != model.StatusCompleted {
		return
	}
	unlock := s.lockObject(obj.URL)
	defer unlock()
	fs, err := s.collectObjectFileSet(task, obj)
	if err != nil {
		s.Logger().Warn("remote-save collect files failed", "task_id", task.ID(), "url", obj.URL, "error", err)
		s.persistState(task, obj, cfg, &model.RemoteSaveState{Status: model.RemoteSaveStatusFailed, Volume: cfg.Volume, Dir: cfg.Dir, LastError: err.Error()})
		return
	}
	if len(fs.files) == 0 {
		s.Logger().Info("remote-save skip object without local files", "task_id", task.ID(), "url", obj.URL)
		return
	}
	if obj.IsCloudDownload() {
		existing := fs.files[:0]
		for _, f := range fs.files {
			if st, err := os.Lstat(f.abs); err == nil && st.Mode().IsRegular() {
				existing = append(existing, f)
			}
		}
		if len(existing) == 0 {
			s.Logger().Info("remote-save skip cloud object without local file", "task_id", task.ID(), "url", obj.URL)
			return
		}
		fs.files = existing
		fs.rels = fs.rels[:0]
		fs.byRel = make(map[string]localFile, len(existing))
		for _, f := range existing {
			if rel, rerr := LocalRel(saveDir(task), f.abs); rerr == nil {
				fs.rels = append(fs.rels, rel)
				fs.byRel[rel] = f
			}
		}
		sort.Strings(fs.rels)
	}
	if state := obj.GetRemoteSave(); !force && state != nil {
		if state.Conflicts > 0 {
			s.Logger().Debug("remote-save skip object with unresolved conflict", "task_id", task.ID(), "url", obj.URL)
			return
		}
		if state.Status == model.RemoteSaveStatusFailed && state.Retries >= s.maxRetries() {
			s.Logger().Warn("remote-save retries exhausted", "task_id", task.ID(), "url", obj.URL, "retries", state.Retries)
			return
		}
	}
	client, err := s.opts.ClientFor(s.sproxyCfg(), cfg.Volume)
	if err != nil {
		s.persistState(task, obj, cfg, &model.RemoteSaveState{Status: model.RemoteSaveStatusFailed, Volume: cfg.Volume, Dir: cfg.Dir, LastError: err.Error()})
		s.Logger().Error("remote-save client init failed", "task_id", task.ID(), "error", err)
		return
	}
	man := s.manifest(task.ID())
	remotePaths := objectRemotePaths(obj, fs.files, fs.rels, cfg)
	for _, rel := range fs.rels {
		f := fs.byRel[rel]
		if IsExcluded(rel, cfg.Exclude) {
			continue
		}
		remote := remotePaths[rel]
		if m, ok := man.Get(rel); ok && m.RemotePath != "" && (m.ObjectURL == "" || m.ObjectURL == obj.URL) {
			remote = m.RemotePath
		}
		if remote == "" {
			continue
		}
		res := s.syncFile(s.ctx, task, client, cfg, man, obj, f, rel, remote, force)
		s.Logger().Info("remote-save file", "task_id", task.ID(), "url", obj.URL, "file", rel, "status", res.Status, "reason", res.Reason)
	}
	_ = man.Flush()
	s.refreshObjectState(task, cfg, obj, man, fs, 1)
}

func (s *Service) syncFile(ctx context.Context, task core.Task, client Client, cfg TaskConfig, man *Manifest, obj *model.DownloadObject, f localFile, rel, remote string, force bool) model.RemoteFileState {
	st, err := os.Lstat(f.abs)
	if err != nil {
		man.Put(ManifestEntry{Path: rel, ObjectURL: obj.URL, RemotePath: remote, Status: model.RemoteFileStatusMissing, Reason: err.Error()})
		return model.RemoteFileState{Path: rel, RemotePath: remote, Status: model.RemoteFileStatusMissing, Reason: err.Error()}
	}
	if !st.Mode().IsRegular() {
		man.Put(ManifestEntry{Path: rel, ObjectURL: obj.URL, RemotePath: remote, Size: st.Size(), MTimeNs: st.ModTime().UnixNano(), Status: model.RemoteFileStatusSkipped, Reason: "not regular file"})
		return model.RemoteFileState{Path: rel, RemotePath: remote, Size: st.Size(), MTime: st.ModTime().UTC(), Status: model.RemoteFileStatusSkipped, Reason: "not regular file"}
	}
	if !force && s.settling(st.ModTime()) {
		man.Put(ManifestEntry{Path: rel, ObjectURL: obj.URL, RemotePath: remote, Size: st.Size(), MTimeNs: st.ModTime().UnixNano(), Status: model.RemoteFileStatusSkipped, Reason: "settling"})
		return model.RemoteFileState{Path: rel, RemotePath: remote, Size: st.Size(), MTime: st.ModTime().UTC(), Status: model.RemoteFileStatusSkipped, Reason: "settling"}
	}
	if entry, ok := man.Get(rel); ok && (entry.ObjectURL == "" || entry.ObjectURL == obj.URL) && !force && !s.verifyChecksum() && entry.Size == st.Size() && entry.MTimeNs == st.ModTime().UnixNano() && entry.SHA256 != "" {
		if entry.Status == model.RemoteFileStatusSkipped && entry.Reason != "settling" {
			return model.RemoteFileState{Path: rel, RemotePath: remote, Size: st.Size(), MTime: st.ModTime().UTC(), SHA256: entry.SHA256, RemoteSHA256: entry.RemoteSHA256, Status: model.RemoteFileStatusSkipped}
		}
		// manifest 只是缓存，不是远端事实源：必须核对远端仍存在且 checksum 未变，
		// 否则远端被删/卷回滚后会永远停留在“已同步”。
		if entry.Status == model.RemoteFileStatusUploaded || entry.Status == model.RemoteFileStatusReused {
			fi, serr := client.Stat(ctx, remote)
			if serr != nil && !errors.Is(serr, sproxyclient.ErrNotFound) {
				return model.RemoteFileState{Path: rel, Status: model.RemoteFileStatusFailed, Reason: "stat: " + serr.Error()}
			}
			if serr == nil && fi != nil && fi.Checksum != "" && (strings.EqualFold(fi.Checksum, entry.RemoteSHA256) || strings.EqualFold(fi.Checksum, entry.SHA256)) {
				meta, merr := BuildMeta(task, obj, rel, remote, st.Size(), st.ModTime(), entry.SHA256, cfg)
				if merr == nil {
					hash, _ := MetaHash(meta)
					if hash == entry.MetaHash {
						return model.RemoteFileState{Path: rel, RemotePath: remote, Size: st.Size(), MTime: st.ModTime().UTC(), SHA256: entry.SHA256, RemoteSHA256: entry.RemoteSHA256, Status: entry.Status}
					}
				}
			}
		}
	}
	sha, herr := hashFile(ctx, f.abs)
	if herr != nil {
		return model.RemoteFileState{Path: rel, Status: model.RemoteFileStatusFailed, Reason: herr.Error()}
	}
	meta, merr := BuildMeta(task, obj, rel, remote, st.Size(), st.ModTime(), sha, cfg)
	if merr != nil {
		return model.RemoteFileState{Path: rel, Status: model.RemoteFileStatusFailed, Reason: merr.Error()}
	}
	metaHash, _ := MetaHash(meta)
	fi, serr := client.Stat(ctx, remote)
	status := model.RemoteFileStatusUploaded
	switch {
	case errors.Is(serr, sproxyclient.ErrNotFound) || (serr == nil && fi == nil):
		sum, uerr := client.Upload(ctx, f.abs, remote)
		if uerr != nil {
			return model.RemoteFileState{Path: rel, Status: model.RemoteFileStatusFailed, Reason: "upload: " + uerr.Error()}
		}
		if !strings.EqualFold(sum, sha) {
			man.Put(ManifestEntry{Path: rel, ObjectURL: obj.URL, RemotePath: remote, Size: st.Size(), MTimeNs: st.ModTime().UnixNano(), SHA256: sha, Status: model.RemoteFileStatusFailed, Reason: "upload checksum mismatch"})
			return model.RemoteFileState{Path: rel, Status: model.RemoteFileStatusFailed, Reason: "upload checksum mismatch"}
		}
		fi, serr = client.Stat(ctx, remote)
		if serr != nil || fi == nil || !strings.EqualFold(fi.Checksum, sha) {
			man.Put(ManifestEntry{Path: rel, ObjectURL: obj.URL, RemotePath: remote, Size: st.Size(), MTimeNs: st.ModTime().UnixNano(), SHA256: sha, Status: model.RemoteFileStatusFailed, Reason: "remote checksum unavailable/mismatch"})
			return model.RemoteFileState{Path: rel, Status: model.RemoteFileStatusFailed, Reason: "remote checksum unavailable/mismatch"}
		}
	case serr != nil:
		return model.RemoteFileState{Path: rel, Status: model.RemoteFileStatusFailed, Reason: "stat: " + serr.Error()}
	case fi.Checksum != "" && strings.EqualFold(fi.Checksum, sha):
		status = model.RemoteFileStatusReused
	default:
		reason := "checksum_mismatch"
		if fi.Checksum == "" {
			reason = "checksum_unknown"
		}
		man.Put(ManifestEntry{Path: rel, ObjectURL: obj.URL, RemotePath: remote, Size: st.Size(), MTimeNs: st.ModTime().UnixNano(), SHA256: sha, RemoteSHA256: fi.Checksum, Status: model.RemoteFileStatusConflict, Reason: reason})
		return model.RemoteFileState{Path: rel, RemotePath: remote, Size: st.Size(), MTime: st.ModTime().UTC(), SHA256: sha, RemoteSHA256: fi.Checksum, Status: model.RemoteFileStatusConflict, Reason: reason}
	}
	if err := client.UpdateMeta(ctx, remote, meta); err != nil {
		man.Put(ManifestEntry{Path: rel, ObjectURL: obj.URL, RemotePath: remote, Size: st.Size(), MTimeNs: st.ModTime().UnixNano(), SHA256: sha, RemoteSHA256: fi.Checksum, Status: model.RemoteFileStatusFailed, Reason: "meta: " + err.Error()})
		return model.RemoteFileState{Path: rel, Status: model.RemoteFileStatusFailed, Reason: "meta: " + err.Error()}
	}
	mi, gerr := client.GetMeta(ctx, remote)
	remoteMTime, perr := time.Parse(time.RFC3339Nano, mi.MTime)
	if gerr != nil || perr != nil || absInt64(remoteMTime.Unix()-st.ModTime().Unix()) > 1 {
		s.Logger().Warn("remote-save meta mtime mismatch", "path", rel, "local", st.ModTime().UTC().Format(time.RFC3339Nano), "remote", mi.MTime, "error", gerr)
		man.Put(ManifestEntry{Path: rel, ObjectURL: obj.URL, RemotePath: remote, Size: st.Size(), MTimeNs: st.ModTime().UnixNano(), SHA256: sha, RemoteSHA256: fi.Checksum, Status: model.RemoteFileStatusFailed, Reason: "meta mtime mismatch"})
		return model.RemoteFileState{Path: rel, Status: model.RemoteFileStatusFailed, Reason: "meta mtime mismatch"}
	}
	entry := ManifestEntry{
		Path: rel, ObjectURL: obj.URL, RemotePath: remote, Size: st.Size(), MTimeNs: st.ModTime().UnixNano(),
		SHA256: sha, RemoteSHA256: fi.Checksum, MetaHash: metaHash, Origin: "local", Status: status,
	}
	man.Put(entry)
	return model.RemoteFileState{Path: rel, RemotePath: remote, Size: st.Size(), MTime: st.ModTime().UTC(), SHA256: sha, RemoteSHA256: fi.Checksum, Status: status}
}

type localFile struct {
	abs string
}

func (s *Service) objectFiles(task core.Task, obj *model.DownloadObject) ([]localFile, error) {
	dir := saveDir(task)
	if dir == "" {
		return nil, fmt.Errorf("任务无 SaveDir")
	}
	var paths []string
	add := func(p string) {
		if strings.TrimSpace(p) != "" {
			paths = append(paths, p)
		}
	}
	add(obj.SavePath)
	add(obj.GetCoverPath())
	add(obj.GetThumbPath())
	add(obj.GetPreviewPath())
	obj.RLock()
	if v, ok := obj.Extra["local_cover"].(string); ok {
		add(v)
	}
	if v, ok := obj.Extra["local_preview"].(string); ok {
		add(v)
	}
	obj.RUnlock()
	if sop, ok := task.(core.SmallObjectProvider); ok {
		for _, info := range sop.SmallObjects(obj) {
			add(info.SavePath)
		}
	}
	seen := map[string]bool{}
	out := make([]localFile, 0, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if seen[abs] {
			continue
		}
		if _, err := LocalRel(dir, abs); err != nil {
			continue
		}
		seen[abs] = true
		out = append(out, localFile{abs: abs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].abs < out[j].abs })
	return out, nil
}

func (s *Service) persistState(task core.Task, obj *model.DownloadObject, _ TaskConfig, state *model.RemoteSaveState) {
	if state.Files == nil && state.Status == model.RemoteSaveStatusFailed && state.LastError == "" {
		return
	}
	if s.opts.IsObjectGone != nil && s.opts.IsObjectGone(task, obj.URL) {
		return
	}
	obj.SetRemoteSave(state)
	if s.opts.Persist != nil {
		s.opts.Persist(task, obj)
	}
}

func (s *Service) lockObject(key string) func() {
	s.objMu.Lock()
	if s.objLocks == nil {
		s.objLocks = map[string]*objectLock{}
	}
	l := s.objLocks[key]
	if l == nil {
		l = &objectLock{}
		s.objLocks[key] = l
	}
	l.refs++
	s.objMu.Unlock()
	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		s.objMu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(s.objLocks, key)
		}
		s.objMu.Unlock()
	}
}

func (s *Service) maxRetries() int {
	if cfg := s.opts.Cfg(); cfg != nil && cfg.RemoteSave.MaxRetries > 0 {
		return cfg.RemoteSave.MaxRetries
	}
	return 3
}

func (s *Service) taskByID(id string) (core.Task, bool) {
	for _, t := range s.opts.Tasks() {
		if t.ID() == id {
			return t, true
		}
	}
	return nil, false
}

func (s *Service) taskConfig(task core.Task) (TaskConfig, error) {
	cfg := s.opts.Cfg()
	if cfg == nil {
		return TaskConfig{}, fmt.Errorf("配置不可用")
	}
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == task.ID() {
			return ParseTaskConfig(cfg.Tasks[i].Extra)
		}
	}
	return TaskConfig{}, nil
}

func (s *Service) sproxyCfg() config.Sproxy {
	if cfg := s.opts.Cfg(); cfg != nil {
		return cfg.Sproxy
	}
	return config.Sproxy{}
}

func (s *Service) manifest(taskID string) *Manifest {
	if v, ok := s.manifests.Load(taskID); ok {
		return v.(*Manifest)
	}
	m, err := LoadManifest(filepath.Join(s.opts.WorkDir, "remote_save"), taskID)
	if err != nil {
		s.Logger().Warn("remote-save manifest load failed", "task_id", taskID, "error", err)
	}
	if m == nil {
		m = &Manifest{entries: map[string]ManifestEntry{}}
	}
	actual, _ := s.manifests.LoadOrStore(taskID, m)
	return actual.(*Manifest)
}

func (s *Service) flushAllManifests() {
	s.manifests.Range(func(_, v any) bool {
		_ = v.(*Manifest).Flush()
		return true
	})
}

func (s *Service) maxFiles() int {
	if cfg := s.opts.Cfg(); cfg != nil && cfg.RemoteSave.MaxFilesPerObject > 0 {
		return cfg.RemoteSave.MaxFilesPerObject
	}
	return 1000
}

func (s *Service) verifyChecksum() bool {
	if cfg := s.opts.Cfg(); cfg != nil {
		return cfg.RemoteSave.VerifyChecksum
	}
	return false
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func (s *Service) settling(mtime time.Time) bool {
	secs := 30
	if cfg := s.opts.Cfg(); cfg != nil && cfg.RemoteSave.SettleSeconds > 0 {
		secs = cfg.RemoteSave.SettleSeconds
	}
	return time.Since(mtime) < time.Duration(secs)*time.Second
}

func (s *Service) Logger() *slog.Logger { return s.opts.Logger }

func saveDir(task core.Task) string {
	if t, ok := task.(interface{ SaveDir() string }); ok {
		return t.SaveDir()
	}
	return ""
}

func hashFile(ctx context.Context, p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 128<<10)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, rerr := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
