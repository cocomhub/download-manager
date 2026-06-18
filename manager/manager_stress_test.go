// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/testutil/assert"
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
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		mgr.Stop(ctx)
		cancel()
		// Wait for goroutines to settle after Stop.
		assert.MustEventually(t, func() bool {
			return runtime.NumGoroutine() <= before+10
		}, 3*time.Second, 50*time.Millisecond, "goroutines did not settle after Start/Stop cycle")
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
		// Brief interleave between concurrent operations.
		time.Sleep(5 * time.Millisecond)
	}
}
