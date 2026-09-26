// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/testutil/assert"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestRetryFailedPermanent_SelectsLeastFailed 验证选批：失败次数最少优先 + 批大小限制。
func TestRetryFailedPermanent_SelectsLeastFailed(t *testing.T) {
	mgr, _ := newMockManager(t, "retry-lf", 5, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)
	task := waitForTask(t, mgr, "retry-lf")

	// 等 5 个对象就绪
	assert.MustEventually(t, func() bool {
		objs, _ := task.Storage().Search(nil)
		return len(objs) >= 5
	}, 3*time.Second, 50*time.Millisecond, "objects seeded")

	// 全部置 failed_permanent
	objs, _ := task.Storage().Search(nil)
	for i, o := range objs {
		task.UpdateStatus(o, model.StatusFailedPermanent, nil)
		// 失败计数：i 越大的失败越多（0,1,2,3,4）
		c := new(atomic.Int64)
		c.Store(int64(i))
		mgr.failedCount.Store(o.URL, c)
	}

	// batch_size=2 → 选失败最少的 2 个（URL 0,1）
	cfg := mgr.currentCfg().Clone()
	cfg.Downloader.Retry = config.RetryConfig{Enabled: true, BatchSize: 2, IntervalHours: 1}
	mgr.configSvc.StoreConfig(cfg)

	n := mgr.retryFailedPermanent()
	if n != 2 {
		t.Fatalf("retryFailedPermanent = %d, want 2", n)
	}

	// 失败最少的 2 个（fails 0,1）被置回 pending——精确断言 URL
	objs, _ = task.Storage().Search(nil)
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
	for _, o := range objs {
		fails := int64(-1)
		if v, ok := mgr.failedCount.Load(o.URL); ok {
			if c, ok := v.(*atomic.Int64); ok {
				fails = c.Load()
			}
		}
		isPending := pendingURLs[o.URL]
		if isPending && fails > 1 {
			t.Fatalf("URL %s (fails=%d) should NOT be retried (least-failed only)", o.URL, fails)
		}
		if !isPending && fails <= 1 {
			t.Fatalf("URL %s (fails=%d) should be retried (least-failed)", o.URL, fails)
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
