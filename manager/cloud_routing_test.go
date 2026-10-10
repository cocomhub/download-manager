// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
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
