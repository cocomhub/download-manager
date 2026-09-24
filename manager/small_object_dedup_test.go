// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

func TestEnqueueSmallObjects_DedupBySavePath(t *testing.T) {
	m := NewManager(&config.Config{Runtime: config.Runtime{Mode: config.RunModeFull}})
	cover := filepath.Join(t.TempDir(), "cover.jpg")
	task := &mockSmallObjectTask{
		id: "test-so",
		smallObjects: []core.SmallObjectInfo{
			{URL: "https://example.com/cover.jpg", SavePath: cover, Rel: "cover"},
		},
	}

	// 两个章节对象共享同一封面 → 第二个对象应被 soInflight 去重，不重复入队。
	tr1 := m.enqueueSmallObjects(task, &model.DownloadObject{URL: "u1", Status: model.StatusPending})
	tr2 := m.enqueueSmallObjects(task, &model.DownloadObject{URL: "u2", Status: model.StatusPending})
	if tr1 == nil || tr2 == nil {
		t.Fatal("expected non-nil trackers")
	}
	if len(m.soQueue) != 1 {
		t.Errorf("soQueue len = %d, want 1 (same SavePath deduped)", len(m.soQueue))
	}
	// 第二个对象的 tracker 应立即完成（不等待兄弟对象的下载）。
	if errs := tr2.WaitAll(time.Second); len(errs) != 0 {
		t.Errorf("tr2 errs = %v", errs)
	}
}

func TestEnqueueSmallObjects_SkipsExistingFile(t *testing.T) {
	m := NewManager(&config.Config{Runtime: config.Runtime{Mode: config.RunModeFull}})
	cover := filepath.Join(t.TempDir(), "cover.jpg")
	if err := os.WriteFile(cover, []byte("img"), 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}
	task := &mockSmallObjectTask{
		id: "test-so",
		smallObjects: []core.SmallObjectInfo{
			{URL: "https://example.com/cover.jpg", SavePath: cover, Rel: "cover"},
		},
	}
	obj := &model.DownloadObject{URL: "u1", Status: model.StatusPending, Extra: map[string]any{}}
	tr := m.enqueueSmallObjects(task, obj)
	if tr == nil {
		t.Fatal("expected non-nil tracker")
	}
	// 文件已存在 → 不入队、不触发网络下载。
	if len(m.soQueue) != 0 {
		t.Errorf("soQueue len = %d, want 0", len(m.soQueue))
	}
	if errs := tr.WaitAll(time.Second); len(errs) != 0 {
		t.Errorf("tracker errs = %v", errs)
	}
	// 媒体字段仍应被写回（finalizeSmallObject 生效）。
	if got := obj.GetMediaURL(model.MediaRelCover); got != "https://example.com/cover.jpg" {
		t.Errorf("cover_url = %q", got)
	}
	if got := obj.GetMediaPath(model.MediaRelCover); got != cover {
		t.Errorf("cover_path = %q", got)
	}
}

func TestEnqueueSmallObjects_QueueFullRecordsPending(t *testing.T) {
	m := NewManager(&config.Config{Runtime: config.Runtime{Mode: config.RunModeFull}})
	cover := filepath.Join(t.TempDir(), "cover.jpg")
	task := &mockSmallObjectTask{
		id: "test-so",
		smallObjects: []core.SmallObjectInfo{
			{URL: "https://example.com/cover.jpg", SavePath: cover, Rel: "cover"},
		},
	}

	// 填满队列（worker 未启动，无人消费）。
	for i := 0; i < cap(m.soQueue); i++ {
		m.soQueue <- smallObjectRequest{tracker: newObjectTracker(1)}
	}
	tr := m.enqueueSmallObjects(task, &model.DownloadObject{URL: "u1", Status: model.StatusPending})
	if tr == nil {
		t.Fatal("expected non-nil tracker")
	}
	key := smallObjectKey("test-so", core.SmallObjectInfo{SavePath: cover})
	// 队列满 → 记入待补集合，不再静默成功。
	if _, ok := m.soPending.Load(key); !ok {
		t.Error("expected pending entry after queue-full drop")
	}
	if errs := tr.WaitAll(time.Second); len(errs) != 1 {
		t.Errorf("tracker errs = %v, want 1 queue-full error", errs)
	}

	// 腾出 1 个空位并 drain → 待补项重新入队、pending 清除。
	<-m.soQueue
	m.drainPendingSO()
	if _, ok := m.soPending.Load(key); ok {
		t.Error("expected pending cleared after drain")
	}
	if len(m.soQueue) != cap(m.soQueue) {
		t.Errorf("queue len = %d, want %d after drain", len(m.soQueue), cap(m.soQueue))
	}
}

// TestProcessSO_NilParentNoPanic 验证补下载请求（parentObj/tracker 为 nil）被 worker 处理时不 panic。
func TestProcessSO_NilParentNoPanic(t *testing.T) {
	m := NewManager(&config.Config{Runtime: config.Runtime{Mode: config.RunModeFull}})
	cover := filepath.Join(t.TempDir(), "cover.jpg")
	if err := os.WriteFile(cover, []byte("img"), 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}
	// parentObj=nil、tracker=nil：走文件已存在跳过分支，finalize 对 nil 安全、tracker 判空。
	m.processSO(smallObjectRequest{
		taskID: "t",
		info:   core.SmallObjectInfo{URL: "https://example.com/cover.jpg", SavePath: cover, Rel: "cover"},
	})
}

func TestDrainPendingSO_SkipsAlreadyDownloaded(t *testing.T) {
	m := NewManager(&config.Config{Runtime: config.Runtime{Mode: config.RunModeFull}})
	cover := filepath.Join(t.TempDir(), "cover.jpg")
	if err := os.WriteFile(cover, []byte("img"), 0o644); err != nil {
		t.Fatalf("write cover: %v", err)
	}
	key := smallObjectKey("test-so", core.SmallObjectInfo{SavePath: cover})
	m.soPending.Store(key, pendingSO{
		taskID: "test-so",
		info:   core.SmallObjectInfo{URL: "https://example.com/cover.jpg", SavePath: cover, Rel: "cover"},
	})

	m.drainPendingSO()
	// 文件已存在 → 直接清除 pending，不重新入队。
	if _, ok := m.soPending.Load(key); ok {
		t.Error("expected pending cleared (file already exists)")
	}
	if len(m.soQueue) != 0 {
		t.Errorf("queue len = %d, want 0 (no re-enqueue for existing file)", len(m.soQueue))
	}
}

// sharedCoverTask 模拟 mxs：多个章节对象共享同一本书的封面 SavePath。
type sharedCoverTask struct {
	*mockTask
	store     core.Storage
	coverPath string
}

func (s *sharedCoverTask) Storage() core.Storage { return s.store }
func (s *sharedCoverTask) SmallObjects(*model.DownloadObject) []core.SmallObjectInfo {
	return []core.SmallObjectInfo{{URL: "https://example.com/cover.jpg", SavePath: s.coverPath, Rel: model.MediaRelCover}}
}
func (s *sharedCoverTask) BackfillDownloadSmallObjects() bool { return true }
