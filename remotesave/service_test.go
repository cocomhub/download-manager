// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	sproxyclient "github.com/cocomhub/sproxy/pkg/client"
	"github.com/cocomhub/sproxy/pkg/sproxysig"
)

type fakeStorage struct {
	mu   sync.Mutex
	objs map[string]*model.DownloadObject
}

func (f *fakeStorage) Get(id string) (*model.DownloadObject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.objs[id]
	if !ok {
		return nil, core.ErrObjectNotFound
	}
	return o, nil
}

func (f *fakeStorage) Update(*model.DownloadObject) error { return nil }
func (f *fakeStorage) Delete(string) error                { return nil }
func (f *fakeStorage) Search(q *core.StorageQuery) ([]*model.DownloadObject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*model.DownloadObject
	for _, o := range f.objs {
		if len(q.Filter.Statuses) > 0 && o.GetStatus() != q.Filter.Statuses[0] {
			continue
		}
		out = append(out, o)
	}
	return out, nil
}
func (f *fakeStorage) Count(*core.StorageQuery) (int64, error) { return int64(len(f.objs)), nil }
func (f *fakeStorage) Exists(ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		_, out[id] = f.objs[id]
	}
	return out, nil
}

type fakeTask struct {
	core.Task
	id      string
	saveDir string
	store   core.Storage
	small   []core.SmallObjectInfo
}

func (f *fakeTask) ID() string                                                { return f.id }
func (f *fakeTask) Type() string                                              { return "hanime" }
func (f *fakeTask) Storage() core.Storage                                     { return f.store }
func (f *fakeTask) SaveDir() string                                           { return f.saveDir }
func (f *fakeTask) SmallObjects(*model.DownloadObject) []core.SmallObjectInfo { return f.small }

type fakeClient struct {
	mu          sync.Mutex
	files       map[string]*FileInfo
	metas       map[string]MetaInfo
	uploads     int
	lastMeta    map[string]any
	uploadErr   error
	afterUpload func(local, remote string) string
}

func (f *fakeClient) Stat(_ context.Context, remote string) (*FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if fi, ok := f.files[remote]; ok {
		return fi, nil
	}
	return nil, sproxyclient.ErrNotFound
}

func (f *fakeClient) Upload(_ context.Context, local, remote string) (string, error) {
	if f.uploadErr != nil {
		f.mu.Lock()
		f.uploads++
		f.mu.Unlock()
		return "", f.uploadErr
	}
	b, err := os.ReadFile(local)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	cs := hex.EncodeToString(sum[:])
	st, _ := os.Stat(local)
	f.mu.Lock()
	f.uploads++
	f.files[remote] = &FileInfo{Size: int64(len(b)), Checksum: f.afterUpload(local, remote)}
	if st != nil {
		f.metas[remote] = MetaInfo{MTime: st.ModTime().UTC().Format(time.RFC3339Nano)}
	}
	f.mu.Unlock()
	return cs, nil
}

func (f *fakeClient) UpdateMeta(_ context.Context, remote string, meta map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastMeta = meta
	return nil
}

func (f *fakeClient) GetMeta(_ context.Context, remote string) (MetaInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.metas[remote], nil
}

func (f *fakeClient) Delete(_ context.Context, remote string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, remote)
	delete(f.metas, remote)
	return nil
}

func (f *fakeClient) Rename(_ context.Context, from, to string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[to] = f.files[from]
	f.metas[to] = f.metas[from]
	delete(f.files, from)
	delete(f.metas, from)
	return nil
}

func newTestService(t *testing.T, task core.Task, client Client, persist func(core.Task, *model.DownloadObject)) *Service {
	t.Helper()
	cfg := &config.Config{
		Sproxy:     config.Sproxy{APIURL: "https://sproxy.example.com"},
		RemoteSave: config.RemoteSaveSettings{SettleSeconds: 1},
		Tasks: []config.Task{{
			ID:    task.ID(),
			Extra: map[string]any{"remote_save": map[string]any{"enabled": true, "volume": "baidu", "dir": "hanime/2.5D"}},
		}},
	}
	s := New(Options{
		Cfg:       func() *config.Config { return cfg },
		Tasks:     func() []core.Task { return []core.Task{task} },
		WorkDir:   t.TempDir(),
		ClientFor: func(config.Sproxy, string) (Client, error) { return client, nil },
		Persist:   persist,
	})
	s.ctx = context.Background()
	return s
}

func TestService_UploadsCompletedObjectAndStoresSHA256(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "video.mp4")
	body := []byte("hello remote save")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	obj := &model.DownloadObject{TaskID: "t1", URL: "u1", SavePath: path, Status: model.StatusCompleted}
	store := &fakeStorage{objs: map[string]*model.DownloadObject{"u1": obj}}
	task := &fakeTask{id: "t1", saveDir: dir, store: store}
	client := &fakeClient{files: map[string]*FileInfo{}, metas: map[string]MetaInfo{}, afterUpload: func(string, string) string {
		sum := sha256.Sum256(body)
		return hex.EncodeToString(sum[:])
	}}
	remote := "hanime/2.5D/video.mp4"
	client.metas[remote] = MetaInfo{MTime: st.ModTime().UTC().Format(time.RFC3339Nano)}
	var persisted *model.DownloadObject
	svc := newTestService(t, task, client, func(_ core.Task, o *model.DownloadObject) { persisted = o })
	// 绕过 settle 窗口：文件已是一小时前。
	svc.handle(request{taskID: "t1", url: "u1"})

	if client.uploads != 1 {
		t.Fatalf("uploads=%d want 1", client.uploads)
	}
	if persisted == nil || persisted.GetRemoteSave() == nil {
		t.Fatal("未持久化 RemoteSave")
	}
	rs := persisted.GetRemoteSave()
	if rs.Status != model.RemoteSaveStatusSynced || len(rs.Files) != 1 {
		t.Fatalf("state=%+v", rs)
	}
	want := sha256.Sum256(body)
	if rs.Files[0].SHA256 != hex.EncodeToString(want[:]) {
		t.Fatalf("mongo sha256=%q want %x", rs.Files[0].SHA256, want)
	}
	if rs.Files[0].Status != model.RemoteFileStatusUploaded {
		t.Fatalf("file status=%q", rs.Files[0].Status)
	}
	if client.lastMeta["x_dm_object"] == nil {
		t.Fatalf("meta 缺少 x_dm_object: %#v", client.lastMeta)
	}
}

func TestService_ConflictWhenRemoteChecksumDiffers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(path, []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	obj := &model.DownloadObject{TaskID: "t1", URL: "u1", SavePath: path, Status: model.StatusCompleted}
	store := &fakeStorage{objs: map[string]*model.DownloadObject{"u1": obj}}
	task := &fakeTask{id: "t1", saveDir: dir, store: store}
	client := &fakeClient{files: map[string]*FileInfo{"hanime/2.5D/video.mp4": {Size: 5, Checksum: "different"}}, metas: map[string]MetaInfo{}}
	var persisted *model.DownloadObject
	svc := newTestService(t, task, client, func(_ core.Task, o *model.DownloadObject) { persisted = o })
	svc.handle(request{taskID: "t1", url: "u1"})
	if client.uploads != 0 {
		t.Fatalf("conflict 不应上传, uploads=%d", client.uploads)
	}
	rs := persisted.GetRemoteSave()
	if rs == nil || rs.Status != model.RemoteSaveStatusConflict || rs.Conflicts != 1 {
		t.Fatalf("state=%+v", rs)
	}
	if rs.Files[0].Reason != "checksum_mismatch" {
		t.Fatalf("reason=%q", rs.Files[0].Reason)
	}
}

func TestService_OnlyCompletedObjectsUploaded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	done := filepath.Join(dir, "done.mp4")
	pending := filepath.Join(dir, "pending.mp4")
	for _, p := range []string{done, pending} {
		if err := os.WriteFile(p, []byte(p), 0o644); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-time.Hour)
		_ = os.Chtimes(p, old, old)
	}
	objDone := &model.DownloadObject{TaskID: "t1", URL: "done", SavePath: done, Status: model.StatusCompleted}
	objPending := &model.DownloadObject{TaskID: "t1", URL: "pending", SavePath: pending, Status: model.StatusPending}
	store := &fakeStorage{objs: map[string]*model.DownloadObject{"done": objDone, "pending": objPending}}
	task := &fakeTask{id: "t1", saveDir: dir, store: store}
	client := &fakeClient{files: map[string]*FileInfo{}, metas: map[string]MetaInfo{}, afterUpload: func(local, _ string) string {
		b, _ := os.ReadFile(local)
		s := sha256.Sum256(b)
		return hex.EncodeToString(s[:])
	}}
	// 为已上传文件返回正确 mtime
	client.metas["hanime/2.5D/done.mp4"] = MetaInfo{MTime: mustStat(t, done).ModTime().UTC().Format(time.RFC3339Nano)}
	svc := newTestService(t, task, client, func(core.Task, *model.DownloadObject) {})
	svc.handle(request{taskID: "t1"})
	if client.uploads != 1 {
		t.Fatalf("只应上传 completed 对象，uploads=%d", client.uploads)
	}
	if objPending.GetRemoteSave() != nil {
		t.Fatal("pending 对象不应有 RemoteSave")
	}
}

func TestService_RequiresRemoteSHA256(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "v.mp4")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(path, old, old)
	obj := &model.DownloadObject{TaskID: "t1", URL: "u", SavePath: path, Status: model.StatusCompleted}
	store := &fakeStorage{objs: map[string]*model.DownloadObject{"u": obj}}
	task := &fakeTask{id: "t1", saveDir: dir, store: store}
	client := &fakeClient{files: map[string]*FileInfo{}, metas: map[string]MetaInfo{}, afterUpload: func(string, string) string { return "" }}
	var persisted *model.DownloadObject
	svc := newTestService(t, task, client, func(_ core.Task, o *model.DownloadObject) { persisted = o })
	svc.handle(request{taskID: "t1", url: "u"})
	rs := persisted.GetRemoteSave()
	if rs == nil || rs.Status != model.RemoteSaveStatusFailed || rs.LastError == "" {
		t.Fatalf("remote sha256 缺失应失败: %+v", rs)
	}
}

func TestService_IgnoresSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	real := filepath.Join(dir, "real.mp4")
	if err := os.WriteFile(real, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.mp4")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlink not supported")
	}
	obj := &model.DownloadObject{TaskID: "t1", URL: "u", SavePath: link, Status: model.StatusCompleted}
	store := &fakeStorage{objs: map[string]*model.DownloadObject{"u": obj}}
	task := &fakeTask{id: "t1", saveDir: dir, store: store}
	client := &fakeClient{files: map[string]*FileInfo{}, metas: map[string]MetaInfo{}}
	var persisted *model.DownloadObject
	svc := newTestService(t, task, client, func(_ core.Task, o *model.DownloadObject) { persisted = o })
	svc.handle(request{taskID: "t1", url: "u"})
	if client.uploads != 0 {
		t.Fatalf("symlink 不应上传, uploads=%d", client.uploads)
	}
	rs := persisted.GetRemoteSave()
	if rs == nil || rs.Status != model.RemoteSaveStatusPending {
		t.Fatalf("symlink 应跳过且对象 pending: %+v", rs)
	}
}

func TestParseTaskConfig_RejectsAccountFields(t *testing.T) {
	t.Parallel()
	_, err := ParseTaskConfig(map[string]any{"remote_save": map[string]any{
		"enabled": true, "volume": "baidu", "dir": "x", "access_key": "ak",
	}})
	if err == nil {
		t.Fatal("任务级账号字段应拒绝")
	}
}

func TestParseTaskConfig_DefaultsDisabled(t *testing.T) {
	t.Parallel()
	cfg, err := ParseTaskConfig(map[string]any{})
	if err != nil || cfg.Enabled {
		t.Fatalf("缺省应 disabled: %+v err=%v", cfg, err)
	}
}

func TestRemotePath_Sanitizes(t *testing.T) {
	t.Parallel()
	cfg := TaskConfig{Dir: "hanime/2.5D"}
	got, err := cfg.RemotePath("x:y?.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hanime/2.5D/x_y_.mp4" {
		t.Fatalf("got %q", got)
	}
	if _, err := cfg.RemotePath("../escape"); err == nil {
		t.Fatal(".. 应拒绝")
	}
}

func TestManifest_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m, err := LoadManifest(dir, "t/1")
	if err != nil {
		t.Fatal(err)
	}
	m.Put(ManifestEntry{Path: "a.mp4", RemotePath: "r/a.mp4", Size: 1, SHA256: "abc", Status: model.RemoteFileStatusUploaded})
	if err := m.Flush(); err != nil {
		t.Fatal(err)
	}
	m2, err := LoadManifest(dir, "t/1")
	if err != nil {
		t.Fatal(err)
	}
	e, ok := m2.Get("a.mp4")
	if !ok || e.SHA256 != "abc" {
		t.Fatalf("entry=%+v ok=%v", e, ok)
	}
}

func mustStat(t *testing.T, p string) os.FileInfo {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestResolveConflict_Overwrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "video.mp4")
	body := []byte("local new content")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(path, old, old)
	obj := &model.DownloadObject{TaskID: "t1", URL: "u1", SavePath: path, Status: model.StatusCompleted}
	store := &fakeStorage{objs: map[string]*model.DownloadObject{"u1": obj}}
	task := &fakeTask{id: "t1", saveDir: dir, store: store}
	client := &fakeClient{
		files: map[string]*FileInfo{"hanime/2.5D/video.mp4": {Size: 5, Checksum: "different"}},
		metas: map[string]MetaInfo{"hanime/2.5D/video.mp4": {MTime: mustStat(t, path).ModTime().UTC().Format(time.RFC3339Nano)}},
		afterUpload: func(string, string) string {
			sum := sha256.Sum256(body)
			return hex.EncodeToString(sum[:])
		},
	}
	svc := newTestService(t, task, client, func(_ core.Task, o *model.DownloadObject) {})
	svc.handle(request{taskID: "t1", url: "u1"})
	if got := obj.GetRemoteSave(); got == nil || got.Status != model.RemoteSaveStatusConflict {
		t.Fatalf("precondition conflict: %+v", got)
	}
	if err := svc.ResolveConflict(context.Background(), "t1", "u1", "video.mp4", ConflictActionOverwrite); err != nil {
		t.Fatal(err)
	}
	got := obj.GetRemoteSave()
	if got == nil || got.Status != model.RemoteSaveStatusSynced {
		t.Fatalf("after overwrite: %+v", got)
	}
	if client.uploads != 1 {
		t.Fatalf("uploads=%d want 1", client.uploads)
	}
}

func TestPrepareCloud_ObjectDirs(t *testing.T) {
	t.Parallel()
	obj := &model.DownloadObject{
		TaskID: "t1", URL: "u1", SavePath: "/tmp/x.mp4",
		Metadata: map[string]string{"code": "ABC-001"}, CloudDownload: true,
	}
	task := &fakeTask{id: "t1", saveDir: "/tmp"}
	cfg := &config.Config{
		Sproxy:     config.Sproxy{APIURL: "https://sproxy.example.com"},
		RemoteSave: config.RemoteSaveSettings{SettleSeconds: 1},
		Tasks: []config.Task{{ID: "t1", Extra: map[string]any{"remote_save": map[string]any{
			"enabled": true, "volume": "baidu", "dir": "hanime/2.5D", "object_dirs": true,
		}}}},
	}
	svc := New(Options{
		Cfg: func() *config.Config { return cfg }, Tasks: func() []core.Task { return []core.Task{task} }, WorkDir: t.TempDir(),
		ClientFor: func(config.Sproxy, string) (Client, error) {
			return &fakeClient{files: map[string]*FileInfo{}, metas: map[string]MetaInfo{}}, nil
		},
	})
	svc.ctx = context.Background()
	if err := svc.PrepareCloud(task, obj); err != nil {
		t.Fatal(err)
	}
	obj.RLock()
	got, _ := obj.Extra["transfer_path"].(string)
	obj.RUnlock()
	if got != "hanime/2.5D/ABC-001/x.mp4" {
		t.Fatalf("transfer_path=%q", got)
	}
}

func TestBuildPlayURL_Verifies(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{Sproxy: config.Sproxy{
		APIURL: "https://sproxy.example.com", PlayBaseURL: "https://sproxy.example.com",
		AccessKey: "ak", AccessKeySecret: "sk", AccessKeyID: "skey-1", PlayTTL: time.Hour,
	}}
	svc := New(Options{Cfg: func() *config.Config { return cfg }})
	play, exp, err := svc.buildPlayURL(&model.RemoteSaveState{RemotePath: "hanime/x.mp4", Volume: "baidu"})
	if err != nil {
		t.Fatal(err)
	}
	if exp.Before(time.Now()) {
		t.Fatal("expired")
	}
	u, err := url.Parse(play)
	if err != nil {
		t.Fatal(err)
	}
	raw := u.Query().Get("spx_sig")
	if raw == "" {
		t.Fatal("missing spx_sig")
	}
	vals := u.Query()
	vals.Del("spx_sig")
	hdr, err := sproxysig.ParseHeader(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := sproxysig.Verify("sk", hdr, sproxysig.Request{Method: "GET", Path: "/download", Query: vals.Encode()}, time.Now(), time.Hour, 0, nil); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestResolveCloudConflict_Reuse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	obj := &model.DownloadObject{
		TaskID: "t1", URL: "u1", SavePath: filepath.Join(dir, "x.mp4"),
		Status: model.StatusCompleted, CloudDownload: true,
		Metadata: map[string]string{"code": "ABC-001"},
		Extra:    map[string]any{"transfer_url": "sproxy://baidu/hanime/2.5D/ABC-001/x.mp4"},
	}
	store := &fakeStorage{objs: map[string]*model.DownloadObject{"u1": obj}}
	task := &fakeTask{id: "t1", saveDir: dir, store: store}
	remote := "hanime/2.5D/ABC-001/x.mp4"
	client := &fakeClient{
		files: map[string]*FileInfo{remote: {Size: 3, Checksum: "abc"}},
		metas: map[string]MetaInfo{remote: {MTime: time.Now().UTC().Format(time.RFC3339Nano)}},
	}
	svc := newTestService(t, task, client, func(_ core.Task, o *model.DownloadObject) {})
	svc.manifest("t1").Put(ManifestEntry{Path: "x.mp4", ObjectURL: "u1", RemotePath: remote, Status: model.RemoteFileStatusConflict, Reason: "target_exists"})
	if err := svc.ResolveConflict(context.Background(), "t1", "u1", "x.mp4", ConflictActionReuse); err != nil {
		t.Fatal(err)
	}
	rs := obj.GetRemoteSave()
	if rs == nil || rs.Status != model.RemoteSaveStatusSynced || rs.RemotePath != remote {
		t.Fatalf("state=%+v", rs)
	}
}

func TestResolveCloudConflict_OverwriteDeletesAndRequeues(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	obj := &model.DownloadObject{
		TaskID: "t1", URL: "u1", SavePath: filepath.Join(dir, "missing.mp4"),
		Status: model.StatusCompleted, CloudDownload: true,
		Metadata: map[string]string{"code": "ABC-001"},
		Extra: map[string]any{
			"transfer_url":  "sproxy://baidu/hanime/2.5D/ABC-001/missing.mp4",
			"cloud_task_id": "old-task",
		},
	}
	store := &fakeStorage{objs: map[string]*model.DownloadObject{"u1": obj}}
	task := &fakeTask{id: "t1", saveDir: dir, store: store}
	remote := "hanime/2.5D/ABC-001/missing.mp4"
	client := &fakeClient{files: map[string]*FileInfo{remote: {Size: 3, Checksum: "abc"}}, metas: map[string]MetaInfo{}}
	cfg := &config.Config{
		Sproxy: config.Sproxy{APIURL: "https://sproxy.example.com"},
		Tasks: []config.Task{{ID: "t1", Extra: map[string]any{"remote_save": map[string]any{
			"enabled": true, "volume": "baidu", "dir": "hanime/2.5D", "object_dirs": true,
		}}}},
	}
	var requeued bool
	svc := New(Options{
		Cfg: func() *config.Config { return cfg }, Tasks: func() []core.Task { return []core.Task{task} },
		WorkDir: t.TempDir(), ClientFor: func(config.Sproxy, string) (Client, error) { return client, nil },
		Requeue: func(core.Task, *model.DownloadObject) { requeued = true },
	})
	svc.ctx = context.Background()
	svc.manifest("t1").Put(ManifestEntry{Path: "missing.mp4", ObjectURL: "u1", RemotePath: remote, Status: model.RemoteFileStatusConflict})
	if err := svc.ResolveConflict(context.Background(), "t1", "u1", "missing.mp4", ConflictActionOverwrite); err != nil {
		t.Fatal(err)
	}
	if !requeued {
		t.Fatal("cloud-only overwrite 应 requeue")
	}
	if obj.GetStatus() != model.StatusPending {
		t.Fatalf("status=%s", obj.GetStatus())
	}
	obj.RLock()
	_, hasTask := obj.Extra["cloud_task_id"]
	obj.RUnlock()
	if hasTask {
		t.Fatal("cloud_task_id 未清理")
	}
	if _, ok := client.files[remote]; ok {
		t.Fatal("远端未删除")
	}
}

func TestMirrorDelete_RemovesOrphan(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "a.mp4")
	if err := os.WriteFile(p, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(p, old, old)
	obj := &model.DownloadObject{TaskID: "t1", URL: "u1", SavePath: p, Status: model.StatusCompleted}
	store := &fakeStorage{objs: map[string]*model.DownloadObject{"u1": obj}}
	task := &fakeTask{id: "t1", saveDir: dir, store: store}
	client := &fakeClient{files: map[string]*FileInfo{}, metas: map[string]MetaInfo{}, afterUpload: func(local, _ string) string {
		b, _ := os.ReadFile(local)
		s := sha256.Sum256(b)
		return hex.EncodeToString(s[:])
	}}
	remote := "hanime/2.5D/a.mp4"
	client.metas[remote] = MetaInfo{MTime: mustStat(t, p).ModTime().UTC().Format(time.RFC3339Nano)}
	cfg := &config.Config{
		Sproxy: config.Sproxy{APIURL: "https://sproxy.example.com"},
		Tasks: []config.Task{{ID: "t1", Extra: map[string]any{"remote_save": map[string]any{
			"enabled": true, "volume": "baidu", "dir": "hanime/2.5D", "mirror_delete": true,
		}}}},
	}
	svc := New(Options{
		Cfg: func() *config.Config { return cfg }, Tasks: func() []core.Task { return []core.Task{task} },
		WorkDir: t.TempDir(), ClientFor: func(config.Sproxy, string) (Client, error) { return client, nil },
	})
	svc.ctx = context.Background()
	svc.handle(request{taskID: "t1"})
	if _, ok := client.files[remote]; !ok {
		t.Fatal("首次同步应上传")
	}
	// 删除对象与本地文件 → 任务级对账应镜像删除远端。
	store.objs = map[string]*model.DownloadObject{}
	_ = os.Remove(p)
	svc.handle(request{taskID: "t1"})
	if _, ok := client.files[remote]; ok {
		t.Fatal("孤儿远端未删除")
	}
}

func TestResolveConflict_NonOverwriteActionsPersist(t *testing.T) {
	t.Parallel()
	for _, action := range []string{ConflictActionSkip, ConflictActionReuse, ConflictActionRename} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			local := filepath.Join(dir, "video.mp4")
			body := []byte("local-content")
			if err := os.WriteFile(local, body, 0o644); err != nil {
				t.Fatal(err)
			}
			old := time.Now().Add(-time.Hour).Truncate(time.Second)
			if err := os.Chtimes(local, old, old); err != nil {
				t.Fatal(err)
			}
			remote := "hanime/2.5D/video.mp4"
			obj := &model.DownloadObject{TaskID: "t1", URL: "u1", SavePath: local, Status: model.StatusCompleted}
			store := &fakeStorage{objs: map[string]*model.DownloadObject{"u1": obj}}
			task := &fakeTask{id: "t1", saveDir: dir, store: store}
			client := &fakeClient{
				files: map[string]*FileInfo{remote: {Size: 13, Checksum: "deadbeef"}},
				metas: map[string]MetaInfo{},
				afterUpload: func(local, _ string) string {
					b, _ := os.ReadFile(local)
					sum := sha256.Sum256(b)
					return hex.EncodeToString(sum[:])
				},
			}
			svc := newTestService(t, task, client, func(core.Task, *model.DownloadObject) {})
			cfg, err := svc.taskConfig(task)
			if err != nil {
				t.Fatal(err)
			}
			svc.syncObject(task, cfg, obj, false)
			if err := svc.ResolveConflict(context.Background(), "t1", "u1", "video.mp4", action); err != nil {
				t.Fatal(err)
			}
			e, ok := svc.manifest("t1").Get("video.mp4")
			if !ok {
				t.Fatal("manifest entry missing")
			}
			want := map[string]string{
				ConflictActionSkip:   model.RemoteFileStatusSkipped,
				ConflictActionReuse:  model.RemoteFileStatusReused,
				ConflictActionRename: model.RemoteFileStatusUploaded,
			}[action]
			if e.Status != want {
				t.Fatalf("status=%s want %s", e.Status, want)
			}
			if action == ConflictActionRename && !strings.Contains(e.RemotePath, "~") {
				t.Fatalf("rename remote=%s", e.RemotePath)
			}
			state := obj.GetRemoteSave()
			if state == nil {
				t.Fatal("state nil")
			}
			if action == ConflictActionSkip && state.Status != model.RemoteSaveStatusSkipped {
				t.Fatalf("object status=%s", state.Status)
			}
		})
	}
}

func TestPrepareCloud_ConflictIsResolvableForFailedPermanentObject(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	obj := &model.DownloadObject{
		TaskID: "t1", URL: "u1", SavePath: filepath.Join(dir, "x.mp4"),
		Metadata: map[string]string{"code": "ABC-001"}, CloudDownload: true,
		Status: model.StatusFailedPermanent,
	}
	store := &fakeStorage{objs: map[string]*model.DownloadObject{"u1": obj}}
	task := &fakeTask{id: "t1", saveDir: dir, store: store}
	target := "hanime/2.5D/ABC-001/x.mp4"
	client := &fakeClient{files: map[string]*FileInfo{target: {Size: 3, Checksum: "old"}}, metas: map[string]MetaInfo{}}
	cfg := &config.Config{
		Sproxy: config.Sproxy{APIURL: "https://sproxy.example.com"},
		Tasks: []config.Task{{ID: "t1", Extra: map[string]any{"remote_save": map[string]any{
			"enabled": true, "volume": "baidu", "dir": "hanime/2.5D", "object_dirs": true,
		}}}},
	}
	svc := New(Options{
		Cfg: func() *config.Config { return cfg }, Tasks: func() []core.Task { return []core.Task{task} },
		WorkDir: t.TempDir(), ClientFor: func(config.Sproxy, string) (Client, error) { return client, nil },
	})
	svc.ctx = context.Background()
	if err := svc.PrepareCloud(task, obj); !errors.Is(err, ErrTargetExists) {
		t.Fatalf("PrepareCloud err=%v", err)
	}
	rel := remoteRelToTarget(TaskConfig{Volume: "baidu", Dir: "hanime/2.5D", ObjectDirs: true}, target)
	if _, ok := svc.manifest("t1").Get(rel); !ok {
		t.Fatal("cloud conflict manifest entry missing")
	}
	// 远端目标冲突对象虽然是 failed_permanent，仍必须允许 reuse。
	if err := svc.ResolveConflict(context.Background(), "t1", "u1", rel, ConflictActionReuse); err != nil {
		t.Fatal(err)
	}
	got := obj.GetRemoteSave()
	if got == nil || got.Status != model.RemoteSaveStatusSynced {
		t.Fatalf("after reuse: %+v", got)
	}
}

func TestMirrorDelete_RemovesStaleFileOfCompletedObject(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.mp4")
	if err := os.WriteFile(keep, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	obj := &model.DownloadObject{TaskID: "t1", URL: "u1", SavePath: keep, Status: model.StatusCompleted}
	store := &fakeStorage{objs: map[string]*model.DownloadObject{"u1": obj}}
	task := &fakeTask{id: "t1", saveDir: dir, store: store}
	svc := newTestService(t, task, &fakeClient{files: map[string]*FileInfo{}, metas: map[string]MetaInfo{}}, nil)
	cfg, err := svc.taskConfig(task)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MirrorDelete = true
	man := svc.manifest("t1")
	man.Put(ManifestEntry{Path: "keep.mp4", ObjectURL: "u1", RemotePath: "hanime/2.5D/keep.mp4", Status: model.RemoteFileStatusUploaded})
	man.Put(ManifestEntry{Path: "stale.mp4", ObjectURL: "u1", RemotePath: "hanime/2.5D/stale.mp4", Status: model.RemoteFileStatusUploaded})
	svc.mirrorCleanup(context.Background(), task, cfg, []*model.DownloadObject{obj}, man)
	if _, ok := man.Get("stale.mp4"); ok {
		t.Fatal("stale manifest entry should be removed")
	}
	if _, ok := man.Get("keep.mp4"); !ok {
		t.Fatal("keep entry removed")
	}
}

func TestMirrorDelete_StorageErrorDoesNotDelete(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// 对象已不在 completed 列表，且 storage.Get 返回非 NotFound 错误：必须 fail-closed。
	store := &errStorage{err: errors.New("mongo unavailable")}
	task := &fakeTask{id: "t1", saveDir: dir, store: store}
	client := &fakeClient{files: map[string]*FileInfo{}, metas: map[string]MetaInfo{}}
	svc := newTestService(t, task, client, nil)
	cfg, _ := svc.taskConfig(task)
	cfg.MirrorDelete = true
	man := svc.manifest("t1")
	man.Put(ManifestEntry{Path: "stale.mp4", ObjectURL: "gone", RemotePath: "hanime/2.5D/stale.mp4", Status: model.RemoteFileStatusUploaded})
	svc.mirrorCleanup(context.Background(), task, cfg, nil, man)
	if _, ok := man.Get("stale.mp4"); !ok {
		t.Fatal("must not delete on storage error")
	}
}

type errStorage struct{ err error }

func (e *errStorage) Get(string) (*model.DownloadObject, error) { return nil, e.err }
func (e *errStorage) Update(*model.DownloadObject) error        { return nil }
func (e *errStorage) Delete(string) error                       { return nil }
func (e *errStorage) Search(*core.StorageQuery) ([]*model.DownloadObject, error) {
	return nil, e.err
}
func (e *errStorage) Count(*core.StorageQuery) (int64, error) { return 0, e.err }
func (e *errStorage) Exists([]string) (map[string]bool, error) {
	return nil, e.err
}

func TestCloudSkip_PersistsAcrossSyncCloud(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	remote := "hanime/2.5D/ABC-001/video.mp4"
	obj := &model.DownloadObject{
		TaskID: "t1", URL: "u1", SavePath: filepath.Join(dir, "video.mp4"),
		Status: model.StatusCompleted, CloudDownload: true,
		Extra: map[string]any{"transfer_url": "local://baidu/" + remote},
	}
	store := &fakeStorage{objs: map[string]*model.DownloadObject{"u1": obj}}
	task := &fakeTask{id: "t1", saveDir: dir, store: store}
	client := &fakeClient{files: map[string]*FileInfo{remote: {Size: 3, Checksum: "abc"}}, metas: map[string]MetaInfo{}}
	svc := newTestService(t, task, client, func(core.Task, *model.DownloadObject) {})
	cfg, _ := svc.taskConfig(task)
	svc.syncCloud(context.Background(), task, cfg, obj)
	if state := obj.GetRemoteSave(); state == nil || state.Status != model.RemoteSaveStatusSynced {
		t.Fatalf("initial cloud state=%+v", state)
	}
	rel := remoteRelToTarget(cfg, remote)
	man := svc.manifest("t1")
	e, _ := man.Get(rel)
	e.Status = model.RemoteFileStatusConflict
	man.Put(e)
	if err := svc.ResolveConflict(context.Background(), "t1", "u1", rel, ConflictActionSkip); err != nil {
		t.Fatal(err)
	}
	if state := obj.GetRemoteSave(); state == nil || state.Status != model.RemoteSaveStatusSkipped {
		t.Fatalf("after skip state=%+v", state)
	}
	// reconcile 的 syncCloud 不得把用户 skip 改回 synced。
	svc.syncCloud(context.Background(), task, cfg, obj)
	if state := obj.GetRemoteSave(); state == nil || state.Status != model.RemoteSaveStatusSkipped {
		t.Fatalf("skip overwritten: %+v", state)
	}
}

func TestSyncFile_ReuploadsWhenRemoteDeleted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	local := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(local, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(local, old, old); err != nil {
		t.Fatal(err)
	}
	obj := &model.DownloadObject{TaskID: "t1", URL: "u1", SavePath: local, Status: model.StatusCompleted}
	task := &fakeTask{id: "t1", saveDir: dir, store: &fakeStorage{objs: map[string]*model.DownloadObject{"u1": obj}}}
	client := &fakeClient{files: map[string]*FileInfo{}, metas: map[string]MetaInfo{}, afterUpload: func(local, _ string) string {
		b, _ := os.ReadFile(local)
		sum := sha256.Sum256(b)
		return hex.EncodeToString(sum[:])
	}}
	svc := newTestService(t, task, client, nil)
	cfg, _ := svc.taskConfig(task)
	svc.syncObject(task, cfg, obj, false)
	if client.uploads != 1 {
		t.Fatalf("uploads=%d", client.uploads)
	}
	remote := "hanime/2.5D/video.mp4"
	if err := client.Delete(context.Background(), remote); err != nil {
		t.Fatal(err)
	}
	svc.syncObject(task, cfg, obj, false)
	if client.uploads != 2 {
		t.Fatalf("remote deletion not repaired: uploads=%d", client.uploads)
	}
}

func TestSyncObject_StopsAfterMaxRetries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	local := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(local, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(local, old, old); err != nil {
		t.Fatal(err)
	}
	obj := &model.DownloadObject{TaskID: "t1", URL: "u1", SavePath: local, Status: model.StatusCompleted}
	task := &fakeTask{id: "t1", saveDir: dir, store: &fakeStorage{objs: map[string]*model.DownloadObject{"u1": obj}}}
	client := &fakeClient{files: map[string]*FileInfo{}, metas: map[string]MetaInfo{}, uploadErr: errors.New("upload down")}
	svc := newTestService(t, task, client, nil)
	svc.opts.Cfg().RemoteSave.MaxRetries = 1
	cfg, _ := svc.taskConfig(task)
	svc.syncObject(task, cfg, obj, false)
	state := obj.GetRemoteSave()
	if state == nil || state.Status != model.RemoteSaveStatusFailed || state.Retries != 1 {
		t.Fatalf("first state=%+v", state)
	}
	svc.syncObject(task, cfg, obj, false)
	if client.uploads != 1 {
		t.Fatalf("retry after max: uploads=%d", client.uploads)
	}
}

func TestMirrorDelete_KeepsCurrentCloudTarget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	remote := "hanime/2.5D/ABC-001/x.mp4"
	obj := &model.DownloadObject{TaskID: "t1", URL: "u1", SavePath: filepath.Join(dir, "x.mp4"), Status: model.StatusCompleted, CloudDownload: true,
		Extra: map[string]any{"transfer_url": "local://baidu/" + remote}}
	task := &fakeTask{id: "t1", saveDir: dir, store: &fakeStorage{objs: map[string]*model.DownloadObject{"u1": obj}}}
	client := &fakeClient{files: map[string]*FileInfo{remote: {Size: 3, Checksum: "abc"}}, metas: map[string]MetaInfo{}}
	svc := newTestService(t, task, client, nil)
	cfg, _ := svc.taskConfig(task)
	cfg.MirrorDelete = true
	man := svc.manifest("t1")
	man.Put(ManifestEntry{Path: remoteRelToTarget(cfg, remote), ObjectURL: "u1", RemotePath: remote, Status: model.RemoteFileStatusUploaded, Origin: "cloud"})
	svc.mirrorCleanup(context.Background(), task, cfg, []*model.DownloadObject{obj}, man)
	if _, ok := man.Get(remoteRelToTarget(cfg, remote)); !ok {
		t.Fatal("current cloud target deleted")
	}
}
