// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
)

// namedDL 是最小的 core.Downloader 假实现，用于验证下载器选择。
type namedDL struct{ name string }

func (d *namedDL) Download(*model.DownloadObject, map[string]string) error { return nil }
func (d *namedDL) Name() string                                            { return d.name }

// TestManagerSelectDownloader_CloudRouting 验证下载项级「云端下载」选项的路由：
// 对象标记云端且云端下载器已配置 → 云端；未标记或未配置 → 默认。
func TestManagerSelectDownloader_CloudRouting(t *testing.T) {
	t.Parallel()
	def := &namedDL{name: "default"}
	cloud := &namedDL{name: "cloud"}

	tests := []struct {
		name      string
		configure bool
		marked    bool
		want      string
	}{
		{name: "未标记_走默认", configure: true, marked: false, want: "default"},
		{name: "标记且已配置_走云端", configure: true, marked: true, want: "cloud"},
		{name: "标记但未配置_回落默认", configure: false, marked: true, want: "default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := &Manager{}
			m.setDownloader(def)
			if tt.configure {
				m.setCloudDownloader(cloud)
			}
			obj := &model.DownloadObject{URL: "https://example.com/x.mp4"}
			obj.SetCloudDownload(tt.marked)
			if got := m.selectDownloader(obj).Name(); got != tt.want {
				t.Fatalf("selectDownloader = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestManagerSelectDownloader_NilObject 验证 nil 对象安全回落默认下载器。
func TestManagerSelectDownloader_NilObject(t *testing.T) {
	t.Parallel()
	m := &Manager{}
	m.setDownloader(&namedDL{name: "default"})
	m.setCloudDownloader(&namedDL{name: "cloud"})
	if got := m.selectDownloader(nil).Name(); got != "default" {
		t.Fatalf("selectDownloader(nil) = %q, want default", got)
	}
}

// TestSetObjectCloudDownload_Manager 验证下载项级云端选项的持久化与读取（含开关往返）。
func TestSetObjectCloudDownload_Manager(t *testing.T) {
	t.Parallel()
	ms, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatalf("NewMemoryStorage: %v", err)
	}
	obj := &model.DownloadObject{TaskID: "t-cd", URL: "http://example.com/cd"}
	obj.SetStatus(model.StatusPending)
	if err := ms.Update(obj); err != nil {
		t.Fatalf("seed object: %v", err)
	}

	m := NewManager(&config.Config{})
	m.tasks.Store("t-cd", &mockTaskWithStorage{id: "t-cd", typ: "mock", st: ms})

	if err := m.SetObjectCloudDownload("t-cd", obj.URL, true); err != nil {
		t.Fatalf("SetObjectCloudDownload(true): %v", err)
	}
	got, err := m.getTaskObject(getTaskFromMgr(t, m, "t-cd"), obj.URL)
	if err != nil || got == nil {
		t.Fatalf("getTaskObject: %v (obj=%v)", err, got)
	}
	if !got.IsCloudDownload() {
		t.Fatal("cloud_download 未持久化")
	}

	if err := m.SetObjectCloudDownload("t-cd", obj.URL, false); err != nil {
		t.Fatalf("SetObjectCloudDownload(false): %v", err)
	}
	got2, err := m.getTaskObject(getTaskFromMgr(t, m, "t-cd"), obj.URL)
	if err != nil || got2 == nil {
		t.Fatalf("getTaskObject(after disable): %v", err)
	}
	if got2.IsCloudDownload() {
		t.Fatal("cloud_download 应已关闭")
	}
}

// TestNewCloudDownloader_Gating 验证云端下载器构建门控与实例复用。
func TestNewCloudDownloader_Gating(t *testing.T) {
	t.Parallel()
	def := &namedDL{name: "default"}

	t.Run("type_sproxy_cloud_复用默认实例", func(t *testing.T) {
		t.Parallel()
		got := newCloudDownloader(config.Downloader{Type: "sproxy_cloud"}, def)
		if got != core.Downloader(def) {
			t.Fatal("type=sproxy_cloud 时应复用默认下载器实例（避免双实例）")
		}
	})
	t.Run("legacy别名_sproxy_hybrid_也复用默认实例", func(t *testing.T) {
		t.Parallel()
		got := newCloudDownloader(config.Downloader{Type: "sproxy_hybrid"}, def)
		if got != core.Downloader(def) {
			t.Fatal("legacy 别名应同样复用默认实例（否则仍构造两个云端实例）")
		}
	})
	t.Run("未配置api_url_返回nil", func(t *testing.T) {
		t.Parallel()
		if got := newCloudDownloader(config.Downloader{}, def); got != nil {
			t.Fatal("未配置 api_url 应为 nil（不启用云端下载）")
		}
	})
	t.Run("配置api_url_新建实例", func(t *testing.T) {
		t.Parallel()
		cfg := config.Downloader{SproxyCloud: config.SproxyCloudConfig{APIURL: "http://127.0.0.1:8080/api/cloud/download"}}
		got := newCloudDownloader(cfg, def)
		if got == nil || got == core.Downloader(def) {
			t.Fatal("配置 api_url 应新建云端下载器实例")
		}
	})
}

// cancelRecDL 记录 Cancel 调用的假下载器。
type cancelRecDL struct {
	name      string
	mu        sync.Mutex
	got       []string
	downloads int
}

func (d *cancelRecDL) Download(*model.DownloadObject, map[string]string) error {
	d.mu.Lock()
	d.downloads++
	d.mu.Unlock()
	return nil
}

func (d *cancelRecDL) downloadCalls() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.downloads
}
func (d *cancelRecDL) Name() string { return d.name }
func (d *cancelRecDL) Cancel(url string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.got = append(d.got, url)
	return nil
}
func (d *cancelRecDL) cancelled() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.got...)
}

// TestCancelObject_RoutesToCloudDownloader 验证按项路由后取消/删除能传到云端下载器
// （否则云端 poll/pullback 持续到 timeout，worker 槽位被占）。
func TestCancelObject_RoutesToCloudDownloader(t *testing.T) {
	t.Parallel()
	ms, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatalf("NewMemoryStorage: %v", err)
	}
	obj := &model.DownloadObject{TaskID: "t-cc", URL: "http://example.com/cc"}
	obj.SetStatus(model.StatusDownloading)
	obj.SetCloudDownload(true)
	if err := ms.Update(obj); err != nil {
		t.Fatalf("seed: %v", err)
	}

	m := NewManager(&config.Config{})
	m.tasks.Store("t-cc", &mockTaskWithStorage{id: "t-cc", typ: "mock", st: ms})
	def := &cancelRecDL{name: "default"}
	cloud := &cancelRecDL{name: "cloud"}
	m.setDownloader(def)
	m.setCloudDownloader(cloud)
	m.downloadingObj.Store(obj.URL, struct{}{})

	if err := m.CancelObject("t-cc", obj.URL); err != nil {
		t.Fatalf("CancelObject: %v", err)
	}
	if got := cloud.cancelled(); len(got) != 1 || got[0] != obj.URL {
		t.Fatalf("云端下载器未收到 Cancel: %v", got)
	}
}

// TestSetObjectCloudDownload_Errors 验证任务/对象不存在时显式报错。
func TestSetObjectCloudDownload_Errors(t *testing.T) {
	t.Parallel()
	m := NewManager(&config.Config{})
	if err := m.SetObjectCloudDownload("nope", "u", true); err == nil {
		t.Fatal("expected task-not-found error")
	}
	ms, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatalf("NewMemoryStorage: %v", err)
	}
	m.tasks.Store("t", &mockTaskWithStorage{id: "t", typ: "mock", st: ms})
	if err := m.SetObjectCloudDownload("t", "missing", true); err == nil {
		t.Fatal("expected object-not-found error")
	}
}

// countingStorage 统计 Update 调用次数（消除 memory 存储「指针别名」导致的持久化断言假绿）。
type countingStorage struct {
	core.Storage
	updates int
}

func (c *countingStorage) Update(obj *model.DownloadObject) error {
	c.updates++
	return c.Storage.Update(obj)
}

// syncRecTask 记录运行时同步调用。
type syncRecTask struct {
	*mockTaskWithStorage
	mu  sync.Mutex
	got []string
}

func (t *syncRecTask) SyncObjectOption(url string, opt model.ObjectOption) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.got = append(t.got, url)
	return true
}

func (t *syncRecTask) synced() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.got...)
}

// TestSetObjectCloudDownload_PersistsAndSyncs 验证：① 真的调用了 Storage().Update（非指针别名假绿）；
// ② 同步了任务运行时对象（mongo 后端存储副本与运行时实例不同指针，否则开关要等重启生效）。
func TestSetObjectCloudDownload_PersistsAndSyncs(t *testing.T) {
	t.Parallel()
	ms, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatalf("NewMemoryStorage: %v", err)
	}
	cs := &countingStorage{Storage: ms}
	obj := &model.DownloadObject{TaskID: "t-ps", URL: "http://example.com/ps"}
	obj.SetStatus(model.StatusPending)
	if err := ms.Update(obj); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tk := &syncRecTask{mockTaskWithStorage: &mockTaskWithStorage{id: "t-ps", typ: "mock", st: cs}}
	m := NewManager(&config.Config{})
	m.tasks.Store("t-ps", tk)

	if err := m.SetObjectCloudDownload("t-ps", obj.URL, true); err != nil {
		t.Fatalf("SetObjectCloudDownload: %v", err)
	}
	if cs.updates == 0 {
		t.Fatal("未调用 Storage().Update（开关不会落库）")
	}
	if got := tk.synced(); len(got) != 1 || got[0] != obj.URL {
		t.Fatalf("未同步运行时对象: %v", got)
	}
}

// TestFeaturesStatus_CloudDownloadBit 验证能力位反映云端下载器是否已配置（UI 据此置灰）。
func TestFeaturesStatus_CloudDownloadBit(t *testing.T) {
	t.Parallel()
	m := &Manager{}
	if m.FeaturesStatus().CloudDownload {
		t.Fatal("未配置云端下载器时能力位应为 false")
	}
	m.setCloudDownloader(&namedDL{name: "cloud"})
	if !m.FeaturesStatus().CloudDownload {
		t.Fatal("已配置云端下载器时能力位应为 true")
	}
}

// TestDownload_RoutesToCloudDownloader 在真实下载入口（m.download）上验证按项路由：
// 标记 cloud_download 的对象必须交给云端下载器（此前仅测 selectDownloader 本体，
// 把 download.go 的调用点改回 getDownloader() 不会红）。
func TestDownload_RoutesToCloudDownloader(t *testing.T) {
	t.Parallel()
	ms, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatalf("NewMemoryStorage: %v", err)
	}
	obj := &model.DownloadObject{
		TaskID:   "t-rt",
		URL:      "http://example.com/rt",
		SavePath: filepath.Join(t.TempDir(), "o.bin"),
	}
	obj.SetStatus(model.StatusPending)
	obj.SetCloudDownload(true)
	if err := ms.Update(obj); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tk := &mockTaskWithStorage{id: "t-rt", typ: "mock", st: ms}
	m := NewManager(&config.Config{})
	m.tasks.Store("t-rt", tk)
	def := &cancelRecDL{name: "default"}
	cloud := &cancelRecDL{name: "cloud"}
	m.setDownloader(def)
	m.setCloudDownloader(cloud)

	m.download(tk, obj)

	if cloud.downloadCalls() != 1 {
		t.Fatalf("云端下载器应被调用 1 次, got %d", cloud.downloadCalls())
	}
	if def.downloadCalls() != 0 {
		t.Fatalf("默认下载器不应被调用, got %d", def.downloadCalls())
	}
}
