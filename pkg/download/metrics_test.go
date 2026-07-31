// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"testing"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
)

func TestMetricRegistryGet(t *testing.T) {
	reg := download.NewMetricRegistry()
	m := reg.Get("http")
	if m == nil {
		t.Fatal("Get('http') returned nil")
	}
	// Same name returns same instance
	m2 := reg.Get("http")
	if m != m2 {
		t.Error("Get should return same instance for same name")
	}
}

func TestMetricRegistryRecord(t *testing.T) {
	reg := download.NewMetricRegistry()
	reg.Record("http", 1024, 100*time.Millisecond, true)
	m := reg.Get("http")

	if m.TotalRequests.Load() != 1 {
		t.Errorf("expected 1 request, got %d", m.TotalRequests.Load())
	}
	if m.TotalBytes.Load() != 1024 {
		t.Errorf("expected 1024 bytes, got %d", m.TotalBytes.Load())
	}
	if m.SuccessCount.Load() != 1 {
		t.Errorf("expected 1 success, got %d", m.SuccessCount.Load())
	}
	if m.FailureCount.Load() != 0 {
		t.Errorf("expected 0 failures, got %d", m.FailureCount.Load())
	}
}

func TestMetricRegistryRecordFail(t *testing.T) {
	reg := download.NewMetricRegistry()
	reg.Record("wget", 0, 5*time.Second, false)
	m := reg.Get("wget")

	if m.TotalRequests.Load() != 1 {
		t.Errorf("expected 1 request, got %d", m.TotalRequests.Load())
	}
	if m.SuccessCount.Load() != 0 {
		t.Errorf("expected 0 successes, got %d", m.SuccessCount.Load())
	}
	if m.FailureCount.Load() != 1 {
		t.Errorf("expected 1 failure, got %d", m.FailureCount.Load())
	}
}

func TestMetricRegistrySnapshot(t *testing.T) {
	reg := download.NewMetricRegistry()
	reg.Record("http", 100, 1*time.Second, true)
	reg.Record("hls", 200, 2*time.Second, false)

	snap := reg.Snapshot()
	if len(snap) != 2 {
		t.Errorf("expected 2 entries in snapshot, got %d", len(snap))
	}

	httpMetrics, ok := snap["http"]
	if !ok {
		t.Fatal("expected 'http' in snapshot")
	}
	if httpMetrics["total_requests"] != 1 {
		t.Errorf("expected 1 total_request, got %d", httpMetrics["total_requests"])
	}
	if httpMetrics["total_bytes"] != 100 {
		t.Errorf("expected 100 total_bytes, got %d", httpMetrics["total_bytes"])
	}
	if httpMetrics["success_count"] != 1 {
		t.Errorf("expected 1 success_count, got %d", httpMetrics["success_count"])
	}
	if httpMetrics["failure_count"] != 0 {
		t.Errorf("expected 0 failure_count, got %d", httpMetrics["failure_count"])
	}
	if httpMetrics["total_duration_ms"] != 1000 {
		t.Errorf("expected 1000 total_duration_ms, got %d", httpMetrics["total_duration_ms"])
	}
	if httpMetrics["last_request_at"] == 0 {
		t.Error("expected last_request_at to be non-zero")
	}

	hlsMetrics, ok := snap["hls"]
	if !ok {
		t.Fatal("expected 'hls' in snapshot")
	}
	if hlsMetrics["total_requests"] != 1 {
		t.Errorf("expected 1 total_request for hls, got %d", hlsMetrics["total_requests"])
	}
	if hlsMetrics["total_bytes"] != 200 {
		t.Errorf("expected 200 total_bytes for hls, got %d", hlsMetrics["total_bytes"])
	}
	if hlsMetrics["success_count"] != 0 {
		t.Errorf("expected 0 success_count for hls, got %d", hlsMetrics["success_count"])
	}
	if hlsMetrics["failure_count"] != 1 {
		t.Errorf("expected 1 failure_count for hls, got %d", hlsMetrics["failure_count"])
	}
}
