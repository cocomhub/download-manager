// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
)

// TestMergeUpgradeResult_PreservesStatusAndProgress 验证升级写回只刷新
// metadata/extra/version，不覆盖并发下载的 status/progress/savepath。
func TestMergeUpgradeResult_PreservesStatusAndProgress(t *testing.T) {
	m := NewManager(&config.Config{})
	st, err := m.newTestStorage()
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}
	url := "https://example.com/x"

	// 存储中的「最新」对象：下载中、有进度。
	latest := &model.DownloadObject{
		URL:      url,
		Status:   model.StatusDownloading,
		Progress: 67,
		SavePath: "/srv/x.mp4",
		Metadata: map[string]string{"title": "old-title", "maker": "old-maker"},
		Extra:    map[string]any{"video_url": "old-url"},
	}
	if err := st.Update(latest); err != nil {
		t.Fatalf("seed latest: %v", err)
	}

	// 升级用的旧快照：metadata 已刷新（date/director/label），但 status 还是旧值 pending。
	upgraded := &model.DownloadObject{
		URL:      url,
		Status:   model.StatusPending,
		Progress: 0,
		SavePath: "",
		Metadata: map[string]string{"title": "new-title", "maker": "new-maker", "date": "2026-09-25", "director": "監督名", "label": "ラベル"},
		Extra:    map[string]any{"video_url": "new-url"},
	}
	upgraded.SetVersion(1)

	if err := mergeUpgradeResult(st, upgraded); err != nil {
		t.Fatalf("mergeUpgradeResult: %v", err)
	}

	got, err := st.Get(url)
	if err != nil {
		t.Fatalf("get after merge: %v", err)
	}
	// 运行态必须保持最新（下载中/进度/路径）。
	if got.GetStatus() != model.StatusDownloading {
		t.Errorf("status overwritten: got %q want %q", got.GetStatus(), model.StatusDownloading)
	}
	if got.GetProgress() != 67 {
		t.Errorf("progress overwritten: got %d want 67", got.GetProgress())
	}
	if got.SavePath != "/srv/x.mp4" {
		t.Errorf("savepath overwritten: got %q", got.SavePath)
	}
	// 抓取数据已刷新。
	if got.Metadata["date"] != "2026-09-25" || got.Metadata["director"] != "監督名" || got.Metadata["label"] != "ラベル" {
		t.Errorf("metadata not refreshed: %+v", got.Metadata)
	}
	if got.Metadata["title"] != "new-title" || got.Metadata["maker"] != "new-maker" {
		t.Errorf("metadata overwrite wrong: %+v", got.Metadata)
	}
	if got.GetVersion() != 1 {
		t.Errorf("version not set: got %d", got.GetVersion())
	}
}

// TestMergeUpgradeResult_ObjectDeleted 升级对象已被删除（Get 失败）→ 回退直接写回，不 panic。
func TestMergeUpgradeResult_ObjectDeleted(t *testing.T) {
	m := NewManager(&config.Config{})
	st, err := m.newTestStorage()
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}
	upgraded := &model.DownloadObject{
		URL:      "https://example.com/gone",
		Status:   model.StatusPending,
		Metadata: map[string]string{"date": "2026-01-01"},
	}
	upgraded.SetVersion(1)
	// 存储中无该对象 → Get 失败 → 直接 Update（upsert 创建）。
	if err := mergeUpgradeResult(st, upgraded); err != nil {
		t.Fatalf("mergeUpgradeResult on missing object: %v", err)
	}
	got, err := st.Get("https://example.com/gone")
	if err != nil || got == nil {
		t.Fatalf("expected upserted object, got err=%v obj=%v", err, got)
	}
}

// newTestStorage 构造内存存储。
func (m *Manager) newTestStorage() (core.Storage, error) {
	return storage.NewMemoryStorage(nil)
}
