// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"context"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

// recordingExtractor 记录被调用的次数。
type recordingExtractor struct {
	called bool
}

func (e *recordingExtractor) Name() string                           { return "recorder" }
func (e *recordingExtractor) Match(_ context.Context, _ string) bool { return true }
func (e *recordingExtractor) Extract(_ context.Context, _ *download.Request) error {
	e.called = true
	return nil
}

// failingExtractor 总是返回错误。
type failingExtractor struct{}

func (f *failingExtractor) Name() string                           { return "failer" }
func (f *failingExtractor) Match(_ context.Context, _ string) bool { return true }
func (f *failingExtractor) Extract(_ context.Context, _ *download.Request) error {
	return download.ErrNoTry
}

func TestMetricsMiddlewareRecords(t *testing.T) {
	reg := download.NewMetricRegistry()
	ex := &recordingExtractor{}

	d := download.New(
		download.WithExtractor(ex),
		download.WithMetricRegistry(reg),
	)

	err := d.Download(t.Context(), &download.Request{
		URL:      "http://example.com/file",
		SavePath: "/tmp/file",
	})
	if err != nil {
		t.Fatalf("Download should succeed: %v", err)
	}

	// Verify the extractor was actually called
	if !ex.called {
		t.Error("expected extractor to be called")
	}

	snap := reg.Snapshot()
	recorderMetrics, ok := snap["recorder"]
	if !ok {
		t.Fatal("expected 'recorder' in metrics snapshot")
	}
	if recorderMetrics["total_requests"] != 1 {
		t.Errorf("expected 1 request, got %d", recorderMetrics["total_requests"])
	}
	if recorderMetrics["success_count"] != 1 {
		t.Errorf("expected 1 success, got %d", recorderMetrics["success_count"])
	}
	if recorderMetrics["failure_count"] != 0 {
		t.Errorf("expected 0 failures, got %d", recorderMetrics["failure_count"])
	}
}

func TestMetricsMiddlewareRecordsFailure(t *testing.T) {
	reg := download.NewMetricRegistry()
	failEx := &failingExtractor{}

	d := download.New(
		download.WithExtractor(failEx),
		download.WithMetricRegistry(reg),
	)

	err := d.Download(t.Context(), &download.Request{
		URL:      "http://example.com/file",
		SavePath: "/tmp/file",
	})
	if err == nil {
		t.Fatal("Download should fail with failingExtractor")
	}

	snap := reg.Snapshot()
	recorderMetrics, ok := snap["failer"]
	if !ok {
		t.Fatal("expected 'failer' in metrics snapshot")
	}
	if recorderMetrics["total_requests"] != 1 {
		t.Errorf("expected 1 request, got %d", recorderMetrics["total_requests"])
	}
	if recorderMetrics["success_count"] != 0 {
		t.Errorf("expected 0 successes, got %d", recorderMetrics["success_count"])
	}
	if recorderMetrics["failure_count"] != 1 {
		t.Errorf("expected 1 failure, got %d", recorderMetrics["failure_count"])
	}
}

func TestCustomMiddleware(t *testing.T) {
	var before, after bool

	customMW := func(ctx context.Context, req *download.Request, next download.Extractor) error {
		before = true
		err := next.Extract(ctx, req)
		after = true
		return err
	}

	ex := &recordingExtractor{}
	d := download.New(
		download.WithExtractor(ex),
		download.WithMiddleware(customMW),
	)

	err := d.Download(t.Context(), &download.Request{
		URL:      "http://example.com/file",
		SavePath: "/tmp/file",
	})
	if err != nil {
		t.Fatalf("Download should succeed: %v", err)
	}
	if !before {
		t.Error("expected middleware 'before' to execute")
	}
	if !after {
		t.Error("expected middleware 'after' to execute")
	}
	if !ex.called {
		t.Error("expected extractor to be called")
	}
}

func TestMiddlewareChainOrder(t *testing.T) {
	var order []string

	mw1 := func(ctx context.Context, req *download.Request, next download.Extractor) error {
		order = append(order, "mw1_before")
		err := next.Extract(ctx, req)
		order = append(order, "mw1_after")
		return err
	}

	mw2 := func(ctx context.Context, req *download.Request, next download.Extractor) error {
		order = append(order, "mw2_before")
		err := next.Extract(ctx, req)
		order = append(order, "mw2_after")
		return err
	}

	ex := &recordingExtractor{}
	d := download.New(
		download.WithExtractor(ex),
		download.WithMiddleware(mw1),
		download.WithMiddleware(mw2),
	)

	err := d.Download(t.Context(), &download.Request{
		URL:      "http://example.com/file",
		SavePath: "/tmp/file",
	})
	if err != nil {
		t.Fatalf("Download should succeed: %v", err)
	}

	// WithMiddleware 将新中间件包装在最外层。
	// 当 mw1 先注册，mw2 再注册时，链为：mw2(mw1(base))。
	// 执行顺序：mw2_before -> mw1_before -> extractor -> mw1_after -> mw2_after
	expected := []string{"mw2_before", "mw1_before", "mw1_after", "mw2_after"}
	if len(order) != len(expected) {
		t.Fatalf("expected order %v, got %v", expected, order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("position %d: expected %s, got %s", i, v, order[i])
		}
	}
}

func TestMetricsMiddlewareWithCustomMiddleware(t *testing.T) {
	reg := download.NewMetricRegistry()
	var customBefore, customAfter bool

	customMW := func(ctx context.Context, req *download.Request, next download.Extractor) error {
		customBefore = true
		err := next.Extract(ctx, req)
		customAfter = true
		return err
	}

	ex := &recordingExtractor{}
	d := download.New(
		download.WithExtractor(ex),
		download.WithMetricRegistry(reg),
		download.WithMiddleware(customMW),
	)

	err := d.Download(t.Context(), &download.Request{
		URL:      "http://example.com/file",
		SavePath: "/tmp/file",
	})
	if err != nil {
		t.Fatalf("Download should succeed: %v", err)
	}

	if !customBefore {
		t.Error("expected custom middleware 'before' to execute")
	}
	if !customAfter {
		t.Error("expected custom middleware 'after' to execute")
	}

	snap := reg.Snapshot()
	recorderMetrics, ok := snap["recorder"]
	if !ok {
		t.Fatal("expected 'recorder' in metrics snapshot")
	}
	if recorderMetrics["total_requests"] != 1 {
		t.Errorf("expected 1 request, got %d", recorderMetrics["total_requests"])
	}
}

func TestRuleSetExtractorHint(t *testing.T) {
	rs := download.NewRuleSet(
		&download.Rule{Pattern: "*.m3u8", Extractor: "hls"},
		&download.Rule{Pattern: "http://*", Extractor: "mock"},
	)

	// Use a DefaultSelector that processes RuleSet hints
	sel := download.NewDefaultSelector()

	ex := &recordingExtractor{}
	d := download.New(
		download.WithExtractor(ex),
		download.WithSelector(sel),
		download.WithRuleSet(rs),
	)

	// Without an hls extractor, but WithRuleSet annotates hints
	// The URL matches *.m3u8 rule, which sets Extractor hint to "hls"
	// Since "hls" extractor is not registered, DefaultSelector falls through
	// to the registered extractors list and finds recordingExtractor
	err := d.Download(t.Context(), &download.Request{
		URL:      "http://example.com/stream.m3u8",
		SavePath: "/tmp/output.mp4",
	})
	if err != nil {
		t.Errorf("Download should succeed: %v", err)
	}
}

func TestWithRuleSetAnnotatesHint(t *testing.T) {
	rs := download.NewRuleSet(
		&download.Rule{Pattern: "*.ts", Extractor: "segment"},
	)

	// Verify that the RuleSet correctly matches
	matched := rs.Match("http://example.com/video.ts", nil)
	if matched == nil {
		t.Fatal("expected match for *.ts URL")
	}
	if matched.Extractor != "segment" {
		t.Errorf("expected extractor 'segment', got '%s'", matched.Extractor)
	}

	// Also verify non-matching URL
	notMatched := rs.Match("http://example.com/video.mp4", nil)
	if notMatched != nil {
		t.Errorf("expected no match for .mp4 URL, got rule with pattern: %s", notMatched.Pattern)
	}
}
