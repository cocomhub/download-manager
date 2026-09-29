// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
	"github.com/cocomhub/download-manager/testutil/assert"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestRetryFailedPermanent_SelectsLeastFailed 验证选批：失败次数最少优先 + 批大小限制。
// 纯单元风格：不启动 worker（worker 并发下载会回写 completed 并清理 failedCount，
// 破坏手动设置的状态/计数，曾致 macOS CI 偶发 flake），直接驱动 retryFailedPermanent。
func TestRetryFailedPermanent_SelectsLeastFailed(t *testing.T) {
	st, err := storage.NewStorage("memory", nil)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	task := &retryStatusTask{id: "retry-lf", typ: "mock", st: st}

	const n = 5
	for i := range n {
		obj := &model.DownloadObject{TaskID: task.ID(), URL: fmt.Sprintf("http://mock-download/file-%d.bin", i)}
		obj.SetStatus(model.StatusFailedPermanent)
		if err := st.Update(obj); err != nil {
			t.Fatalf("seed object: %v", err)
		}
	}

	mgr := NewManager(&config.Config{
		Server: config.Server{WorkDir: t.TempDir()},
		Downloader: config.Downloader{
			GlobalConcurrent: 5,
			MaxRetries:       2,
			Retry:            config.RetryConfig{Enabled: true, BatchSize: 2, IntervalHours: 1},
		},
	})
	mgr.tasks.Store(task.ID(), task)

	// 失败计数：i 越大的失败越多（0,1,2,3,4）
	for i := range n {
		c := new(atomic.Int64)
		c.Store(int64(i))
		mgr.failedCount.Store(fmt.Sprintf("http://mock-download/file-%d.bin", i), c)
	}

	// batch_size=2 → 选失败最少的 2 个（URL 0,1）
	if got := mgr.retryFailedPermanent(); got != 2 {
		t.Fatalf("retryFailedPermanent = %d, want 2", got)
	}

	// 失败最少的 2 个（fails 0,1）被置回 pending——精确断言 URL
	objs, err := st.Search(nil)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(objs) != n {
		t.Fatalf("objects = %d, want %d", len(objs), n)
	}
	pendingURLs := map[string]bool{}
	for _, o := range objs {
		if o.GetStatus() == model.StatusPending {
			pendingURLs[o.URL] = true
		}
	}
	if len(pendingURLs) != 2 {
		t.Fatalf("pending after retry = %d, want 2 (least-failed batch)", len(pendingURLs))
	}
	// 验证选中的是 fails=0 和 fails=1 的对象（file-0.bin / file-1.bin）
	for i := range n {
		url := fmt.Sprintf("http://mock-download/file-%d.bin", i)
		isPending := pendingURLs[url]
		if isPending && i > 1 {
			t.Fatalf("URL %s (fails=%d) should NOT be retried (least-failed only)", url, i)
		}
		if !isPending && i <= 1 {
			t.Fatalf("URL %s (fails=%d) should be retried (least-failed)", url, i)
		}
	}
}

// TestRetryFailedPermanent_Disabled 验证 enabled=false 不重试。
func TestRetryFailedPermanent_Disabled(t *testing.T) {
	mgr, _ := newMockManager(t, "retry-off", 2, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "retry-off")

	assert.MustEventually(t, func() bool {
		objs, _ := task.Storage().Search(nil)
		return len(objs) >= 2
	}, 3*time.Second, 50*time.Millisecond, "objects seeded")

	objs, _ := task.Storage().Search(nil)
	for _, o := range objs {
		task.UpdateStatus(o, model.StatusFailedPermanent, nil)
	}

	cfg := mgr.currentCfg().Clone()
	cfg.Downloader.Retry = config.RetryConfig{Enabled: false}
	mgr.configSvc.StoreConfig(cfg)

	if n := mgr.retryFailedPermanent(); n != 0 {
		t.Fatalf("retryFailedPermanent with disabled = %d, want 0", n)
	}
}
