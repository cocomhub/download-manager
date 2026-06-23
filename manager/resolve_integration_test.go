// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// ---- mockSmallObjectTask 瀹炵幇浜?core.Task + core.SmallObjectProvider ----

type mockSmallObjectTask struct {
	id           string
	typ          string
	objs         []*model.DownloadObject
	storage      core.Storage
	resolved     map[string]bool
	resolveErr   error
	smallObjects []core.SmallObjectInfo
}

func (t *mockSmallObjectTask) ID() string                            { return t.id }
func (t *mockSmallObjectTask) Type() string                          { return t.typ }
func (t *mockSmallObjectTask) Logger() *slog.Logger                  { return slog.Default() }
func (t *mockSmallObjectTask) Storage() core.Storage                 { return t.storage }
func (t *mockSmallObjectTask) SetDownloader(core.Downloader)         {}
func (t *mockSmallObjectTask) GetDownloadHeaders() map[string]string { return nil }
func (t *mockSmallObjectTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	return t.objs, nil
}
func (t *mockSmallObjectTask) UpdateStatus(obj *model.DownloadObject, status string, _ error) error {
	obj.SetStatus(status)
	return nil
}
func (t *mockSmallObjectTask) Concurrency() int             { return 3 }
func (t *mockSmallObjectTask) SetConcurrency(int) error     { return nil }
func (t *mockSmallObjectTask) RefreshInterval() int         { return 60 }
func (t *mockSmallObjectTask) SetRefreshInterval(int) error { return nil }
func (t *mockSmallObjectTask) Start() error                 { return nil }
func (t *mockSmallObjectTask) ResolveObject(_ context.Context, obj *model.DownloadObject) error {
	if t.resolveErr != nil {
		return t.resolveErr
	}
	t.resolved[obj.URL] = true
	obj.Extra["files"] = []map[string]string{
		{"url": obj.URL + "/video.mp4", "path": obj.SavePath + "/video.mp4", "type": "video"},
	}
	return nil
}
func (t *mockSmallObjectTask) Close() error { return nil }
func (t *mockSmallObjectTask) SmallObjects(obj *model.DownloadObject) []core.SmallObjectInfo {
	return t.smallObjects
}
func (t *mockSmallObjectTask) MarkAsFailed(*model.DownloadObject, error) {}
func (t *mockSmallObjectTask) IsMarkedFailed(string) bool                { return false }

// ---- Tests ----

func TestEnqueueResolve(t *testing.T) {
	m := NewManager(&config.Config{Runtime: config.Runtime{Mode: config.RunModeFull}})
	obj := &model.DownloadObject{URL: "https://example.com/video1", Status: model.StatusPending}

	m.enqueueResolve("test-task", obj)

	// Should be enqueued (non-blocking), verify no panic
	select {
	case req := <-m.resolveQueue:
		if req.taskID != "test-task" {
			t.Errorf("expected taskID test-task, got %s", req.taskID)
		}
		if req.obj.URL != "https://example.com/video1" {
			t.Errorf("expected url video1, got %s", req.obj.URL)
		}
	default:
		t.Error("expected resolve queue to have item")
	}
}

func TestEnqueueSmallObjects(t *testing.T) {
	m := NewManager(&config.Config{Runtime: config.Runtime{Mode: config.RunModeFull}})
	task := &mockSmallObjectTask{
		id: "test-so",
		smallObjects: []core.SmallObjectInfo{
			{URL: "https://example.com/thumb.jpg", SavePath: "/tmp/thumb.jpg", Rel: "cover"},
			{URL: "https://example.com/preview.mp4", SavePath: "/tmp/preview.mp4", Rel: "preview"},
		},
	}
	obj := &model.DownloadObject{URL: "https://example.com/video1", Status: model.StatusPending}

	tracker := m.enqueueSmallObjects(task, obj)
	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}

	if tracker.total != 2 {
		t.Errorf("expected 2 small objects, got %d", tracker.total)
	}

	// Verify items in queue
	select {
	case req := <-m.soQueue:
		if req.info.Rel != "cover" {
			t.Errorf("expected cover, got %s", req.info.Rel)
		}
	default:
		t.Error("expected small object queue to have item")
	}
}

func TestHasFiles(t *testing.T) {
	tests := []struct {
		name     string
		obj      *model.DownloadObject
		expected bool
	}{
		{"nil obj", nil, false},
		{"nil extra", &model.DownloadObject{Extra: nil}, false},
		{"empty extra", &model.DownloadObject{Extra: map[string]any{}}, false},
		{"files empty slice", &model.DownloadObject{Extra: map[string]any{"files": []any{}}}, false},
		{"files with items", &model.DownloadObject{Extra: map[string]any{"files": []any{"a"}}}, true},
		{"files map slice", &model.DownloadObject{Extra: map[string]any{"files": []map[string]string{{"url": "a"}}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasFiles(tt.obj); got != tt.expected {
				t.Errorf("hasFiles() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestObjectTracker(t *testing.T) {
	tracker := newObjectTracker(2)

	go func() {
		tracker.MarkDone(nil)
		tracker.MarkDone(nil)
	}()

	errs := tracker.WaitAll(5 * time.Second)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestObjectTrackerWithErrors(t *testing.T) {
	tracker := newObjectTracker(2)

	go func() {
		tracker.MarkDone(nil)
		tracker.MarkDone(fmt.Errorf("download failed"))
	}()

	errs := tracker.WaitAll(5 * time.Second)
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
}

func TestObjectTrackerTimeout(t *testing.T) {
	tracker := newObjectTracker(2)

	// Never mark done 鈥?should timeout without panic, errs should be nil (no errors collected)
	errs := tracker.WaitAll(10 * time.Millisecond)
	if len(errs) != 0 {
		t.Errorf("expected 0 errs on timeout with no MarkDone calls, got %d", len(errs))
	}
}

func TestProcessTaskSkipsResolving(t *testing.T) {
	m := NewManager(&config.Config{Runtime: config.Runtime{Mode: config.RunModeFull}})
	obj := &model.DownloadObject{URL: "https://example.com/video", Status: model.StatusResolving}
	task := &mockSmallObjectTask{id: "test", objs: []*model.DownloadObject{obj}}

	m.tasks.Store("test", task)
	m.processTask(task)

	// 涓嶅簲鍏ラ槦锛屼笉搴旀敼鍙樼姸鎬?	select {
	case <-m.resolveQueue:
		t.Error("should not enqueue a resolving object")
	default:
	}
	if obj.GetStatus() != model.StatusResolving {
		t.Errorf("expected StatusResolving, got %s", obj.GetStatus())
	}
}

func TestProcessTaskEnqueuesResolve(t *testing.T) {
	m := NewManager(&config.Config{Runtime: config.Runtime{Mode: config.RunModeFull}})
	obj := &model.DownloadObject{URL: "https://example.com/video", Status: model.StatusPending}
	task := &mockSmallObjectTask{
		id:       "test",
		objs:     []*model.DownloadObject{obj},
		resolved: make(map[string]bool),
	}

	m.tasks.Store("test", task)
	m.processTask(task)

	// 搴斿叆闃?resolve
	select {
	case req := <-m.resolveQueue:
		if req.obj.URL != "https://example.com/video" {
			t.Errorf("expected video URL, got %s", req.obj.URL)
		}
	default:
		t.Error("expected resolve queue to have item")
	}

	// 鐘舵€佸簲涓?Resolving
	if obj.GetStatus() != model.StatusResolving {
		t.Errorf("expected StatusResolving, got %s", obj.GetStatus())
	}
}
