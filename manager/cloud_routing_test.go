// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
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
	name string
	mu   sync.Mutex
	got  []string
}

func (d *cancelRecDL) Download(*model.DownloadObject, map[string]string) error { return nil }
func (d *cancelRecDL) Name() string                                            { return d.name }
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
