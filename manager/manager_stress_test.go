// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// TestManagerStartStop_GoroutineLeak verifies Start/Stop does not leak goroutines.
func TestManagerStartStop_GoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	t.Logf("goroutines before: %d", before)

	for range 5 {
		mgr := NewManager(&config.Config{
			Server: config.Server{WorkDir: t.TempDir()},
		})
		go mgr.Start()
		<-mgr.Initialized()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		mgr.Stop(ctx)
		cancel()
		time.Sleep(200 * time.Millisecond)
	}

	after := runtime.NumGoroutine()
	t.Logf("goroutines after: %d", after)

	leaked := after - before
	if leaked > 10 {
		t.Errorf("possible goroutine leak: %d goroutines remained after 5 Start/Stop cycles (before: %d, after: %d)",
			leaked, before, after)
	}
}

// TestScheduler_ConcurrentAccess verifies concurrent safety under multiple scan rounds.
func TestScheduler_ConcurrentAccess(t *testing.T) {
	mgr, _ := newMockManager(t, "stress", 50,
		mockdl.New(mockdl.ModeAlwaysSuccess, mockdl.WithDelay(10*time.Millisecond)))
	_ = startManager(t, mgr) // registers Stop cleanup via t.Cleanup

	for range 20 {
		mgr.scan()
		mgr.AggregateObjects(1, 100, "", "created_at", "", nil)
		mgr.GetHealthStatus()
		time.Sleep(5 * time.Millisecond)
	}
}
