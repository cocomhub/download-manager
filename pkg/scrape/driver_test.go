// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package scrape

import (
	"context"
	"errors"
	"testing"
)

var errFake = errors.New("fake error for testing")

// memoryTracker implements Tracker in-memory for tests.
type memoryTracker struct {
	succ     map[string]bool
	progress map[string]ProgressInfo
}

func newMemoryTracker() *memoryTracker {
	return &memoryTracker{
		succ:     make(map[string]bool),
		progress: make(map[string]ProgressInfo),
	}
}

func (m *memoryTracker) IsFullSucceeded(taskID string) bool { return m.succ[taskID] }
func (m *memoryTracker) MarkFullSucceeded(taskID string) error {
	m.succ[taskID] = true
	return nil
}
func (m *memoryTracker) DeleteFullSuccess(taskID string) error {
	delete(m.succ, taskID)
	return nil
}
func (m *memoryTracker) LoadProgress(taskID string) (ProgressInfo, bool) {
	info, ok := m.progress[taskID]
	return info, ok
}
func (m *memoryTracker) SaveProgress(taskID string, info ProgressInfo) error {
	m.progress[taskID] = info
	return nil
}
func (m *memoryTracker) ClearProgress(taskID string) error {
	delete(m.progress, taskID)
	return nil
}

func newMockPager(result Result) *mockPager {
	return &mockPager{result: result}
}

type mockPager struct {
	result Result
}

func (m *mockPager) Run(ctx context.Context, hooks PageHooks, opts Options) Result {
	return m.result
}

func TestDriver_ColdStart_FullSuccess(t *testing.T) {
	tracker := newMemoryTracker()
	pager := newMockPager(Result{
		Items:         []any{"a", "b"},
		AllSucceeded:  true,
		DetectedPages: 3,
	})
	driver := NewDriver(tracker, pager)

	hooks := PageHooks{
		BuildPageURL:    func(page int) string { return "http://mock/page/" + itoa(page) },
		RunScraper:      func(url string) (string, error) { return "<html></html>", nil },
		ParseTotalPages: func(html string) int { return 3 },
		ParsePage:       func(html string) (any, error) { return []string{}, nil },
		ProcessItems:    func(items any) ([]any, bool) { return nil, true },
	}

	result := driver.Scrape(t.Context(), "test-id", hooks, Options{})
	if !result.AllSucceeded {
		t.Fatal("Expected AllSucceeded=true")
	}
	if !tracker.IsFullSucceeded("test-id") {
		t.Fatal("Expected tracker to have full_succeeded")
	}
}

func TestDriver_ColdStart_PartialFail_ProgressSaved(t *testing.T) {
	tracker := newMemoryTracker()
	pager := newMockPager(Result{
		AllSucceeded:   false,
		LastFailedPage: 3,
		DetectedPages:  10,
	})
	driver := NewDriver(tracker, pager)

	hooks := PageHooks{
		BuildPageURL:    func(page int) string { return "http://mock/page/" + itoa(page) },
		RunScraper:      func(url string) (string, error) { return "", errFake },
		ParseTotalPages: func(html string) int { return 10 },
		ParsePage:       func(html string) (any, error) { return nil, nil },
		ProcessItems:    func(items any) ([]any, bool) { return nil, false },
	}

	driver.Scrape(t.Context(), "test-id", hooks, Options{})
	if tracker.IsFullSucceeded("test-id") {
		t.Fatal("Expected NOT full_succeeded after failure")
	}
	info, ok := tracker.LoadProgress("test-id")
	if !ok {
		t.Fatal("Expected progress saved")
	}
	if info.LastFailedPage != 3 {
		t.Fatalf("Expected LastFailedPage=3, got %d", info.LastFailedPage)
	}
}

func TestDriver_Incremental_FailDowngrade(t *testing.T) {
	tracker := newMemoryTracker()
	_ = tracker.MarkFullSucceeded("test-id") // start in incremental mode

	pager := newMockPager(Result{
		AllSucceeded:   false,
		LastFailedPage: 2,
		DetectedPages:  15,
	})
	driver := NewDriver(tracker, pager)

	hooks := PageHooks{
		BuildPageURL:    func(page int) string { return "http://mock/page/" + itoa(page) },
		RunScraper:      func(url string) (string, error) { return "", errFake },
		ParseTotalPages: func(html string) int { return 15 },
		ParsePage:       func(html string) (any, error) { return nil, nil },
		ProcessItems:    func(items any) ([]any, bool) { return nil, false },
	}

	driver.Scrape(t.Context(), "test-id", hooks, Options{})
	if tracker.IsFullSucceeded("test-id") {
		t.Fatal("Expected full_succeeded to be DELETED after incremental failure")
	}
	info, ok := tracker.LoadProgress("test-id")
	if !ok {
		t.Fatal("Expected progress saved after incremental failure")
	}
	if info.LastFailedPage != 2 {
		t.Fatalf("Expected LastFailedPage=2, got %d", info.LastFailedPage)
	}
}

func TestDriver_Incremental_SuccessKeepsFullSucc(t *testing.T) {
	tracker := newMemoryTracker()
	_ = tracker.MarkFullSucceeded("test-id")

	pager := newMockPager(Result{
		Items:        []any{"new-item"},
		AllSucceeded: true,
	})
	driver := NewDriver(tracker, pager)

	hooks := PageHooks{
		BuildPageURL:    func(page int) string { return "http://mock/page/1" },
		RunScraper:      func(url string) (string, error) { return "<html></html>", nil },
		ParseTotalPages: func(html string) int { return 10 },
		ParsePage:       func(html string) (any, error) { return []string{}, nil },
		ProcessItems:    func(items any) ([]any, bool) { return nil, true },
	}

	result := driver.Scrape(t.Context(), "test-id", hooks, Options{})
	if !result.AllSucceeded {
		t.Fatal("Expected AllSucceeded=true")
	}
	if !tracker.IsFullSucceeded("test-id") {
		t.Fatal("Expected full_succeeded to remain intact after incremental success")
	}
}

func TestDriver_ResumeFromProgress(t *testing.T) {
	tracker := newMemoryTracker()
	_ = tracker.SaveProgress("test-id", ProgressInfo{LastFailedPage: 5, MaxDetectedPage: 10})

	tracker2 := newMemoryTracker()
	// Copy progress
	tracker2.progress["test-id"] = ProgressInfo{LastFailedPage: 5, MaxDetectedPage: 10}

	pager := newMockPager(Result{
		Items:         []any{"c"},
		AllSucceeded:  true,
		DetectedPages: 10,
	})
	driver := NewDriver(tracker2, pager)

	hooks := PageHooks{
		BuildPageURL:    func(page int) string { return "http://mock/page/" + itoa(page) },
		RunScraper:      func(url string) (string, error) { return "<html></html>", nil },
		ParseTotalPages: func(html string) int { return 10 },
		ParsePage:       func(html string) (any, error) { return []string{}, nil },
		ProcessItems:    func(items any) ([]any, bool) { return nil, true },
	}

	driver.Scrape(t.Context(), "test-id", hooks, Options{})
	if !tracker2.IsFullSucceeded("test-id") {
		t.Fatal("Expected full_succeeded after resuming from progress and completing")
	}
	_, ok := tracker2.LoadProgress("test-id")
	if ok {
		t.Fatal("Expected progress to be cleared after full success")
	}
}

// TestDriver_MaxPagesCapResume 验证 max_pages 截断后下次从 max_pages+1 续传：
// 1. 第一次跑 MaxPages=5 → 截断 → tracker 记录 LastFailedPage=6
// 2. 第二次跑（模拟重启）→ applyScrapeMode 从 6 续传
func TestDriver_MaxPagesCapResume(t *testing.T) {
	tracker := newMemoryTracker()
	var startPages []int
	var pageIndex int

	hooks := PageHooks{
		BuildPageURL:    func(page int) string { return "http://mock/page/" + itoa(page) },
		RunScraper:      func(url string) (string, error) { return "<html></html>", nil },
		ParseTotalPages: func(html string) int { return -1 }, // 无总数 → max_pages 截断
		ParsePage:       func(html string) (any, error) { pageIndex++; return []any{"x"}, nil },
		ProcessItems:    func(items any) ([]any, bool) { return []any{"x"}, false }, // 每页新对象 → 持续分页
	}

	// 第一次：MaxPages=5 截断
	pager := NewDefaultPager()
	driver := NewDriver(tracker, pager)
	driver.Scrape(t.Context(), "test-id", hooks, Options{MaxPages: 5})
	if tracker.IsFullSucceeded("test-id") {
		t.Fatal("max_pages cap stop should NOT mark full success")
	}
	info, ok := tracker.LoadProgress("test-id")
	if !ok || info.LastFailedPage != 6 {
		t.Fatalf("expected progress LastFailedPage=6, got %+v ok=%v", info, ok)
	}

	// 第二次：模拟重启 → 从 6 续传
	tracker2 := newMemoryTracker()
	_ = tracker2.SaveProgress("test-id", info)
	pager2 := NewDefaultPager()
	driver2 := NewDriver(tracker2, pager2)
	driver2.Scrape(t.Context(), "test-id", hooks, Options{MaxPages: 10})
	if len(startPages) > 0 {
		_ = startPages // keep compile
	}
	// 第二次应从 page 6 开始扫（applyScrapeMode 用 LastFailedPage）
	// 用 pageIndex 从 0 开始累计两次 —— 第一次扫 5 页，第二次从 6 起扫到 10
	if pageIndex != 10 {
		t.Fatalf("expected total 10 pages (5 + 5 resumed), got %d", pageIndex)
	}
	// 第二次 MaxPages=10 从 6 起扫到 10（5 页）→ 又截断 → 记录 LastFailedPage=11
	if tracker2.IsFullSucceeded("test-id") {
		t.Fatal("second run with MaxPages=10 starting at 6 should also cap (6-10), not full success")
	}
	info2, ok2 := tracker2.LoadProgress("test-id")
	if !ok2 || info2.LastFailedPage != 11 {
		t.Fatalf("expected progress LastFailedPage=11 after resume cap, got %+v ok=%v", info2, ok2)
	}
}
