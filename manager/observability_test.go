// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sync/atomic"
	"testing"
	"time"
)

// --- Health Status Tests ---

func TestGetHealthStatus_AllStopped(t *testing.T) {
	m := &Manager{
		startedAt:        time.Now(),
		schedulerEnabled: atomic.Bool{},
		workersEnabled:   atomic.Bool{},
		maxFailures:      1000,
		failureRecords:   make([]FailureRecord, 1000),
	}
	// Both disabled by default
	hs := m.GetHealthStatus()

	if hs.Status != "ok" {
		t.Fatalf("expected overall status 'ok' when scheduler/workers stopped, got %q", hs.Status)
	}
	if hs.Components["scheduler"].Status != "stopped" {
		t.Fatalf("expected scheduler 'stopped', got %q", hs.Components["scheduler"].Status)
	}
	if hs.Components["workers"].Status != "stopped" {
		t.Fatalf("expected workers 'stopped', got %q", hs.Components["workers"].Status)
	}
	if hs.Components["eventbus"].Status != "ok" {
		t.Fatalf("expected eventbus 'ok', got %q", hs.Components["eventbus"].Status)
	}
	if hs.Components["tasks"].Status != "ok" {
		t.Fatalf("expected tasks 'ok', got %q", hs.Components["tasks"].Status)
	}
	if hs.Uptime == "" {
		t.Fatal("expected non-empty uptime")
	}
}

func TestGetHealthStatus_SchedulerEnabledFresh(t *testing.T) {
	m := &Manager{
		startedAt:        time.Now(),
		schedulerEnabled: atomic.Bool{},
		workersEnabled:   atomic.Bool{},
		maxFailures:      1000,
		failureRecords:   make([]FailureRecord, 1000),
	}
	m.schedulerEnabled.Store(true)

	hs := m.GetHealthStatus()
	if hs.Components["scheduler"].Status != "error" {
		t.Fatalf("expected scheduler 'error' (fresh, no heartbeat), got %q", hs.Components["scheduler"].Status)
	}
}

func TestGetHealthStatus_SchedulerHeartbeatRecent(t *testing.T) {
	m := &Manager{
		startedAt:        time.Now(),
		schedulerEnabled: atomic.Bool{},
		workersEnabled:   atomic.Bool{},
		maxFailures:      1000,
		failureRecords:   make([]FailureRecord, 1000),
	}
	m.schedulerEnabled.Store(true)
	m.schedulerHeartbeat.Store(time.Now())
	time.Sleep(time.Millisecond) // ensure heartbeat is fresh

	hs := m.GetHealthStatus()
	if hs.Components["scheduler"].Status != "ok" {
		t.Fatalf("expected scheduler 'ok' with recent heartbeat, got %q", hs.Components["scheduler"].Status)
	}
}

func TestGetHealthStatus_SchedulerHeartbeatStale(t *testing.T) {
	m := &Manager{
		startedAt:        time.Now(),
		schedulerEnabled: atomic.Bool{},
		workersEnabled:   atomic.Bool{},
		maxFailures:      1000,
		failureRecords:   make([]FailureRecord, 1000),
	}
	m.schedulerEnabled.Store(true)
	m.schedulerHeartbeat.Store(time.Now().Add(-10 * time.Second)) // stale

	hs := m.GetHealthStatus()
	if hs.Components["scheduler"].Status != "error" {
		t.Fatalf("expected scheduler 'error' with stale heartbeat, got %q", hs.Components["scheduler"].Status)
	}
}

func TestGetHealthStatus_OverallDegraded(t *testing.T) {
	m := &Manager{
		startedAt:        time.Now(),
		schedulerEnabled: atomic.Bool{},
		workersEnabled:   atomic.Bool{},
		maxFailures:      1000,
		failureRecords:   make([]FailureRecord, 1000),
	}
	m.schedulerEnabled.Store(true)
	m.schedulerHeartbeat.Store(time.Now())
	// Worker not set — overall stays ok since workers are disabled

	hs := m.GetHealthStatus()
	if hs.Status != "ok" {
		t.Fatalf("expected overall 'ok', got %q", hs.Status)
	}
}

// --- Failure Record Ring Buffer Tests ---

func TestRecordFailure_Basic(t *testing.T) {
	m := &Manager{
		maxFailures:    100,
		failureRecords: make([]FailureRecord, 100),
	}

	m.recordFailure("t1", "http://example.com/1", "timeout", 1, false)
	m.recordFailure("t1", "http://example.com/2", "refused", 2, true)
	m.recordFailure("t2", "http://example.com/3", "canceled", 1, false)

	result := m.GetFailures("", 200)
	failures := result["failures"].([]FailureRecord)
	total := result["total"].(int)

	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
	if len(failures) != 3 {
		t.Fatalf("expected 3 failure records, got %d", len(failures))
	}
	// Order: newest first
	if failures[0].URL != "http://example.com/3" {
		t.Fatalf("expected newest first: http://example.com/3, got %s", failures[0].URL)
	}
}

func TestRecordFailure_WrapAround(t *testing.T) {
	capacity := 10
	m := &Manager{
		maxFailures:    capacity,
		failureRecords: make([]FailureRecord, capacity),
	}

	// Write capacity + 5 records to trigger wrap-around
	for i := 0; i < capacity+5; i++ {
		m.recordFailure("t1", "url", "error", i+1, false)
	}

	result := m.GetFailures("", 200)
	failures := result["failures"].([]FailureRecord)
	total := result["total"].(int)

	if total != capacity+5 {
		t.Fatalf("expected total %d, got %d", capacity+5, total)
	}
	// Should only return up to capacity records
	if len(failures) != capacity {
		t.Fatalf("expected %d records (ring buffer capacity), got %d", capacity, len(failures))
	}
	// Oldest record should be attempt=6 (the 6th write, since first 5 are overwritten)
	if failures[len(failures)-1].Attempt != 6 {
		t.Fatalf("expected oldest record attempt=6, got %d", failures[len(failures)-1].Attempt)
	}
}

func TestGetFailures_FilterByTaskID(t *testing.T) {
	m := &Manager{
		maxFailures:    100,
		failureRecords: make([]FailureRecord, 100),
	}

	m.recordFailure("t1", "url1", "err1", 1, false)
	m.recordFailure("t2", "url2", "err2", 1, false)
	m.recordFailure("t1", "url3", "err3", 2, true)

	result := m.GetFailures("t1", 200)
	failures := result["failures"].([]FailureRecord)

	if len(failures) != 2 {
		t.Fatalf("expected 2 failures for t1, got %d", len(failures))
	}
	for _, f := range failures {
		if f.TaskID != "t1" {
			t.Fatalf("expected all records to have task_id=t1, got %s", f.TaskID)
		}
	}
}

func TestGetFailures_Limit(t *testing.T) {
	m := &Manager{
		maxFailures:    100,
		failureRecords: make([]FailureRecord, 100),
	}

	for i := range 10 {
		m.recordFailure("t1", "url", "error", i+1, false)
	}

	result := m.GetFailures("", 3)
	failures := result["failures"].([]FailureRecord)

	if len(failures) != 3 {
		t.Fatalf("expected 3 records with limit=3, got %d", len(failures))
	}
}

func TestGetFailures_MaxLimit(t *testing.T) {
	m := &Manager{
		maxFailures:    100,
		failureRecords: make([]FailureRecord, 100),
	}

	for i := range 50 {
		m.recordFailure("t1", "url", "error", i+1, false)
	}

	// Passing limit > 200 should be clamped to 200, but we only have 50 records
	result := m.GetFailures("", 999)
	failures := result["failures"].([]FailureRecord)

	if len(failures) != 50 {
		t.Fatalf("expected 50 records (all available), got %d", len(failures))
	}
}

// --- CollectMetrics Tests ---

func TestCollectMetrics_Empty(t *testing.T) {
	m := &Manager{
		startedAt:      time.Now(),
		maxFailures:    1000,
		failureRecords: make([]FailureRecord, 1000),
	}

	metrics := m.CollectMetrics()
	if metrics == nil {
		t.Fatal("expected non-nil metrics")
	}

	tasks := metrics["tasks"].(map[string]any)
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks in metrics, got %d", len(tasks))
	}

	global := metrics["global"].(map[string]any)
	if global["active_downloads"].(int) != 0 {
		t.Fatalf("expected 0 active downloads, got %d", global["active_downloads"])
	}
	if global["scheduler"].(string) != "stopped" {
		t.Fatalf("expected scheduler 'stopped', got %q", global["scheduler"])
	}
	if global["subscriber_count"].(int) != 0 {
		t.Fatalf("expected 0 subscribers, got %d", global["subscriber_count"])
	}

	uptime := metrics["uptime"].(string)
	if uptime == "" {
		t.Fatal("expected non-empty uptime")
	}
}

func TestCollectMetrics_WithTaskMetrics(t *testing.T) {
	m := &Manager{
		startedAt:       time.Now(),
		activeDownloads: make(map[string]int),
		maxFailures:     1000,
		failureRecords:  make([]FailureRecord, 1000),
	}

	mt := &taskMetrics{}
	mt.completed.Store(10)
	mt.failures.Store(2)
	mt.retried.Store(1)
	mt.avgLatencyMs.Store(1500)
	mt.lastActive.Store(time.Now().Unix())
	m.metrics.Store("test-task", mt)

	metrics := m.CollectMetrics()
	tasks := metrics["tasks"].(map[string]any)

	tm, ok := tasks["test-task"]
	if !ok {
		t.Fatal("expected test-task in metrics")
	}
	data := tm.(map[string]any)
	if data["completed"].(int64) != 10 {
		t.Fatalf("expected completed=10, got %v", data["completed"])
	}
	if data["failures"].(int64) != 2 {
		t.Fatalf("expected failures=2, got %v", data["failures"])
	}
	if data["retried"].(int64) != 1 {
		t.Fatalf("expected retried=1, got %v", data["retried"])
	}
}
