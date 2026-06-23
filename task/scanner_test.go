// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"errors"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/model"
)

// mockAdapter implements SiteAdapter for testing.
type mockAdapter struct {
	buildPageURLFn    func(page int) string
	runScraperFn      func(url string) (string, error)
	parseTotalPagesFn func(html string) int
	parsePageFn       func(html string) (any, error)
	itemsToURLsFn     func(items any) []string
	buildObjectFn     func(items any, index int) (*model.DownloadObject, error)
	resolveDetailFn   func(obj *model.DownloadObject) error
	getHeadersFn      func() map[string]string
}

func (m *mockAdapter) BuildPageURL(page int) string          { return m.buildPageURLFn(page) }
func (m *mockAdapter) RunScraper(url string) (string, error) { return m.runScraperFn(url) }
func (m *mockAdapter) ParseTotalPages(html string) int       { return m.parseTotalPagesFn(html) }
func (m *mockAdapter) ParsePage(html string) (any, error)    { return m.parsePageFn(html) }
func (m *mockAdapter) ItemsToURLs(items any) []string        { return m.itemsToURLsFn(items) }
func (m *mockAdapter) BuildObject(items any, i int) (*model.DownloadObject, error) {
	return m.buildObjectFn(items, i)
}
func (m *mockAdapter) ResolveDetail(obj *model.DownloadObject) error { return m.resolveDetailFn(obj) }
func (m *mockAdapter) GetDownloadHeaders() map[string]string         { return m.getHeadersFn() }

func newTestBaseTask(t *testing.T) *BaseTask {
	t.Helper()
	bt, err := NewBaseTask(&config.Task{
		ID:      "test",
		Type:    "test",
		Storage: config.StorageConfig{Type: "memory"},
	}, Options{})
	if err != nil {
		t.Fatalf("NewBaseTask failed: %v", err)
	}
	return bt
}

func TestPagingScanner_Run_NoDriver(t *testing.T) {
	bt := newTestBaseTask(t)
	adapter := &mockAdapter{
		buildPageURLFn: func(page int) string { return "" },
	}
	s := NewPagingScanner(bt, adapter)
	if err := s.Run(t.Context()); err != nil {
		t.Fatalf("expected nil error with no driver, got %v", err)
	}
}

func TestPagingScanner_processItems_AllNew(t *testing.T) {
	bt := newTestBaseTask(t)
	items := []string{"url1", "url2"}
	adapter := &mockAdapter{
		itemsToURLsFn: func(items any) []string {
			return items.([]string)
		},
		buildObjectFn: func(items any, index int) (*model.DownloadObject, error) {
			urls := items.([]string)
			return &model.DownloadObject{URL: urls[index], TaskID: "test"}, nil
		},
	}
	s := NewPagingScanner(bt, adapter)
	newItems, allKnown := s.processItems(items)
	if allKnown {
		t.Fatal("expected allKnown=false for new items")
	}
	if len(newItems) != 2 {
		t.Fatalf("expected 2 new items, got %d", len(newItems))
	}
}

func TestPagingScanner_processItems_AllKnown(t *testing.T) {
	bt := newTestBaseTask(t)
	bt.RememberRuntimeObject(&model.DownloadObject{URL: "url1", TaskID: "test"}, true)
	bt.RememberRuntimeObject(&model.DownloadObject{URL: "url2", TaskID: "test"}, true)

	adapter := &mockAdapter{
		itemsToURLsFn: func(items any) []string { return items.([]string) },
		buildObjectFn: func(items any, i int) (*model.DownloadObject, error) {
			return nil, errors.New("should not be called")
		},
	}
	s := NewPagingScanner(bt, adapter)
	newItems, allKnown := s.processItems([]string{"url1", "url2"})
	if !allKnown {
		t.Fatal("expected allKnown=true when all URLs already known")
	}
	if len(newItems) != 0 {
		t.Fatalf("expected 0 new items, got %d", len(newItems))
	}
}

func TestPagingScanner_processItems_PartialKnown(t *testing.T) {
	bt := newTestBaseTask(t)
	bt.RememberRuntimeObject(&model.DownloadObject{URL: "url1", TaskID: "test"}, true)

	adapter := &mockAdapter{
		itemsToURLsFn: func(items any) []string { return items.([]string) },
		buildObjectFn: func(items any, index int) (*model.DownloadObject, error) {
			urls := items.([]string)
			return &model.DownloadObject{URL: urls[index], TaskID: "test"}, nil
		},
	}
	s := NewPagingScanner(bt, adapter)
	newItems, allKnown := s.processItems([]string{"url1", "url2"})
	if allKnown {
		t.Fatal("expected allKnown=false when some URLs unknown")
	}
	if len(newItems) != 1 {
		t.Fatalf("expected 1 new item, got %d", len(newItems))
	}
}

func TestPagingScanner_buildHooks(t *testing.T) {
	bt := newTestBaseTask(t)
	adapter := &mockAdapter{
		buildPageURLFn:    func(page int) string { return "page1" },
		runScraperFn:      func(url string) (string, error) { return "html", nil },
		parseTotalPagesFn: func(html string) int { return 5 },
		parsePageFn:       func(html string) (any, error) { return []string{"a"}, nil },
	}
	s := NewPagingScanner(bt, adapter)
	hooks := s.buildHooks()
	if hooks.BuildPageURL(1) != "page1" {
		t.Fatal("BuildPageURL hook mismatch")
	}
	html, _ := hooks.RunScraper("x")
	if html != "html" {
		t.Fatal("RunScraper hook mismatch")
	}
	if hooks.ParseTotalPages("x") != 5 {
		t.Fatal("ParseTotalPages hook mismatch")
	}
	items, err := hooks.ParsePage("x")
	if err != nil || len(items.([]string)) != 1 {
		t.Fatal("ParsePage hook mismatch")
	}
}
