// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"log/slog"
	"testing"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
)

// TestCategorizeFailure 验证失败分类启发式（基于错误串派生 4xx/5xx/超时/其它）。
func TestCategorizeFailure(t *testing.T) {
	cases := []struct {
		name      string
		err       string
		permanent bool
		want      string
	}{
		{"404", "HTTP 404", true, FailureCategoryHTTP4xx},
		{"403 wrapped", "download failed: HTTP 403 Forbidden", false, FailureCategoryHTTP4xx},
		{"500", "HTTP 500", false, FailureCategoryHTTP5xx},
		{"503 wrapped", "HTTP 503 Service Unavailable", false, FailureCategoryHTTP5xx},
		{"deadline", "context deadline exceeded", false, FailureCategoryTimeout},
		{"timeout word", "mock download timeout", false, FailureCategoryTimeout},
		{"other refused", "mock download failed", false, FailureCategoryOther},
		{"max retries HTTP 500 permanent", "max retries reached: HTTP 500", true, FailureCategoryOther},
		{"max retries HTTP 404 permanent", "max retries reached: HTTP 403", true, FailureCategoryHTTP4xx},
		{"empty", "", false, FailureCategoryOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := categorizeFailure(tc.err, tc.permanent)
			if got != tc.want {
				t.Fatalf("categorizeFailure(%q) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// TestRetryOverview_Aggregates 验证聚合统计：分类计数 + total/retryable/permanent。
func TestRetryOverview_Aggregates(t *testing.T) {
	m := &Manager{
		maxFailures:    100,
		failureRecords: make([]FailureRecord, 100),
	}
	m.recordFailure("t1", "http://x/1", "HTTP 404", 2, true)                   // http_4xx, permanent
	m.recordFailure("t1", "http://x/2", "HTTP 500", 1, false)                  // http_5xx
	m.recordFailure("t2", "http://x/3", "context deadline exceeded", 1, false) // timeout
	m.recordFailure("t2", "http://x/4", "refused", 1, false)                   // other

	ov := m.RetryOverview()
	if ov.TotalFailed != 4 {
		t.Fatalf("TotalFailed = %d, want 4", ov.TotalFailed)
	}
	if ov.Retryable != 3 {
		t.Fatalf("Retryable = %d, want 3", ov.Retryable)
	}
	if ov.Permanent != 1 {
		t.Fatalf("Permanent = %d, want 1", ov.Permanent)
	}
	wantCat := map[string]int{
		FailureCategoryHTTP4xx: 1,
		FailureCategoryHTTP5xx: 1,
		FailureCategoryTimeout: 1,
		FailureCategoryOther:   1,
	}
	for cat, n := range wantCat {
		if ov.Categories[cat] != n {
			t.Fatalf("category %q = %d, want %d", cat, ov.Categories[cat], n)
		}
	}
}

// TestRetryOverview_Empty 验证无记录时返回全零（默认分类键存在）。
func TestRetryOverview_Empty(t *testing.T) {
	m := &Manager{
		maxFailures:    10,
		failureRecords: make([]FailureRecord, 10),
	}
	ov := m.RetryOverview()
	if ov.TotalFailed != 0 {
		t.Fatalf("TotalFailed = %d, want 0", ov.TotalFailed)
	}
	for _, cat := range []string{FailureCategoryHTTP4xx, FailureCategoryHTTP5xx, FailureCategoryTimeout, FailureCategoryOther} {
		if _, ok := ov.Categories[cat]; !ok {
			t.Fatalf("expected default key %q in categories", cat)
		}
	}
}

// retryStatusTask 是支持真实 UpdateStatus 的测试任务（把状态落到对象再写存储），
// 用于验证 RetryAllFailedStatus 跨任务重置失败对象。
type retryStatusTask struct {
	id   string
	typ  string
	st   core.Storage
	objs []*model.DownloadObject
}

func (r *retryStatusTask) ID() string                            { return r.id }
func (r *retryStatusTask) Type() string                          { return r.typ }
func (r *retryStatusTask) Logger() *slog.Logger                  { return slog.Default() }
func (r *retryStatusTask) Storage() core.Storage                 { return r.st }
func (r *retryStatusTask) SetDownloader(core.Downloader)         {}
func (r *retryStatusTask) GetDownloadHeaders() map[string]string { return nil }
func (r *retryStatusTask) GetAllObjects(lock bool) []*model.DownloadObject {
	return r.objs
}
func (r *retryStatusTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	obj.SetStatus(status)
	if r.st != nil {
		return r.st.Update(obj)
	}
	return nil
}
func (r *retryStatusTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	return nil, nil
}
func (r *retryStatusTask) Concurrency() int             { return 1 }
func (r *retryStatusTask) SetConcurrency(int) error     { return nil }
func (r *retryStatusTask) RefreshInterval() int         { return 0 }
func (r *retryStatusTask) SetRefreshInterval(int) error { return nil }
func (r *retryStatusTask) Start() error                 { return nil }
func (r *retryStatusTask) ResolveObject(_ context.Context, _ *model.DownloadObject) error {
	return nil
}
func (r *retryStatusTask) Close() error { return nil }

// TestRetryAllFailedStatus 验证跨任务按状态批量重置失败对象为 pending。
func TestRetryAllFailedStatus(t *testing.T) {
	st, err := storage.NewStorage("memory", nil)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	failed := &model.DownloadObject{TaskID: "t1", URL: "http://x/a"}
	failed.SetStatus("failed")
	perm := &model.DownloadObject{TaskID: "t2", URL: "http://x/b"}
	perm.SetStatus("failed_permanent")
	done := &model.DownloadObject{TaskID: "t2", URL: "http://x/c"}
	done.SetStatus("completed")
	for _, o := range []*model.DownloadObject{failed, perm, done} {
		if err := st.Update(o); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	t1 := &retryStatusTask{id: "t1", typ: "mock", st: st, objs: []*model.DownloadObject{failed}}
	t2 := &retryStatusTask{id: "t2", typ: "mock", st: st, objs: []*model.DownloadObject{perm, done}}

	m := &Manager{
		maxFailures:     100,
		failureRecords:  make([]FailureRecord, 100),
		activeDownloads: make(map[string]int),
		schedulerSignal: make(chan struct{}, 1),
	}
	m.tasks.Store("t1", t1)
	m.tasks.Store("t2", t2)

	// 只重置 failed（可重试），失败的对象从 failed_permanent 各自归类。
	n, err := m.RetryAllFailedStatus([]string{model.StatusFailed})
	if err != nil {
		t.Fatalf("RetryAllFailedStatus: %v", err)
	}
	if n != 1 {
		t.Fatalf("retried = %d, want 1 (仅 failed)", n)
	}
	if failed.GetStatus() != model.StatusPending {
		t.Fatalf("failed obj status = %q, want pending", failed.GetStatus())
	}
}

// guardRetryStatusTask 在 retryStatusTask 基础上实现 TaskStatusGuarder，
// 用于验证 RetryAllFailedStatus 尊重并发取消的 cancelled 状态。
type guardRetryStatusTask struct {
	*retryStatusTask
	refuse bool
	calls  int
}

func (t *guardRetryStatusTask) SetStatusUnlessCancelled(obj *model.DownloadObject, status string, err error) bool {
	t.calls++
	if t.refuse {
		return false // 模拟并发 CancelObject 已把对象置 cancelled
	}
	return t.UpdateStatus(obj, status, err) == nil
}

// TestRetryAllFailedStatus_RespectsGuard 验证 RetryAllFailedStatus 用
// SetStatusUnlessCancelled 守卫：守卫拒绝时跳过重置、不计数、不改状态。
func TestRetryAllFailedStatus_RespectsGuard(t *testing.T) {
	st, err := storage.NewStorage("memory", nil)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	failed := &model.DownloadObject{TaskID: "t1", URL: "http://x/guard"}
	failed.SetStatus(model.StatusFailed)
	if err := st.Update(failed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	refused := &guardRetryStatusTask{retryStatusTask: &retryStatusTask{
		id: "t1", typ: "mock", st: st, objs: []*model.DownloadObject{failed},
	}, refuse: true}
	m := &Manager{
		maxFailures:     100,
		failureRecords:  make([]FailureRecord, 100),
		activeDownloads: make(map[string]int),
		schedulerSignal: make(chan struct{}, 1),
	}
	m.tasks.Store("t1", refused)

	n, err := m.RetryAllFailedStatus([]string{model.StatusFailed})
	if err != nil {
		t.Fatalf("RetryAllFailedStatus: %v", err)
	}
	if n != 0 {
		t.Fatalf("retried = %d, want 0 when guard refuses (cancelled)", n)
	}
	if failed.GetStatus() != model.StatusFailed {
		t.Errorf("object status changed to %q despite guard refusing", failed.GetStatus())
	}
	if refused.calls != 1 {
		t.Errorf("guard calls = %d, want 1", refused.calls)
	}

	// 守卫放行 → 正常重置。
	failed2 := &model.DownloadObject{TaskID: "t2", URL: "http://x/guard2"}
	failed2.SetStatus(model.StatusFailed)
	if err := st.Update(failed2); err != nil {
		t.Fatalf("seed guard2: %v", err)
	}
	allowed := &guardRetryStatusTask{retryStatusTask: &retryStatusTask{
		id: "t2", typ: "mock", st: st, objs: []*model.DownloadObject{failed2},
	}, refuse: false}
	m2 := &Manager{
		maxFailures:     100,
		failureRecords:  make([]FailureRecord, 100),
		activeDownloads: make(map[string]int),
		schedulerSignal: make(chan struct{}, 1),
	}
	m2.tasks.Store("t2", allowed)

	n2, err := m2.RetryAllFailedStatus([]string{model.StatusFailed})
	if err != nil {
		t.Fatalf("RetryAllFailedStatus (allowed): %v", err)
	}
	if n2 != 1 {
		t.Fatalf("retried = %d, want 1 when guard allows", n2)
	}
	if failed2.GetStatus() != model.StatusPending {
		t.Errorf("object status = %q, want pending when guard allows", failed2.GetStatus())
	}
}
