// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
)

func TestFeaturesStatus_UIOnly(t *testing.T) {
	cfg := &config.Config{}
	cfg.Runtime.Mode = config.RunModeUI
	cfg.Runtime.Download.Enabled = true
	cfg.Runtime.Scheduler.Enabled = true
	m := NewManager(cfg)
	done := make(chan struct{})
	go func() {
		m.Start()
		close(done)
	}()
	time.Sleep(150 * time.Millisecond)
	fs := m.FeaturesStatus()
	if fs.Scheduler || fs.Workers {
		t.Fatalf("expected scheduler=false, workers=false in ui mode, got scheduler=%v workers=%v", fs.Scheduler, fs.Workers)
	}
	m.Stop(context.Background())
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("manager did not stop in time")
	}
}

func TestFeaturesStatus_Full(t *testing.T) {
	cfg := &config.Config{}
	cfg.Runtime.Mode = config.RunModeFull
	cfg.Runtime.Download.Enabled = true
	cfg.Runtime.Scheduler.Enabled = true
	m := NewManager(cfg)
	done := make(chan struct{})
	go func() {
		m.Start()
		close(done)
	}()
	time.Sleep(150 * time.Millisecond)
	fs := m.FeaturesStatus()
	if !fs.Scheduler || !fs.Workers {
		t.Fatalf("expected scheduler=true, workers=true in full mode, got scheduler=%v workers=%v", fs.Scheduler, fs.Workers)
	}
	m.Stop(context.Background())
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("manager did not stop in time")
	}
}
