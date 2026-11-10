// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	sproxyclient "github.com/cocomhub/sproxy/pkg/client"
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
	b, err := os.ReadFile(local)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	cs := hex.EncodeToString(sum[:])
	f.mu.Lock()
	f.uploads++
	f.files[remote] = &FileInfo{Size: int64(len(b)), Checksum: f.afterUpload(local, remote)}
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
	if rs == nil || rs.Status != model.RemoteSaveStatusSynced {
		t.Fatalf("symlink 应跳过且对象 synced: %+v", rs)
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
