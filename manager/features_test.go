// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/testutil/assert"
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
	assert.MustEventually(t, func() bool {
		select {
		case <-m.Initialized():
			return true
		default:
			return false
		}
	}, 3*time.Second, 50*time.Millisecond, "wait for manager initialization")
	fs := m.FeaturesStatus()
	if fs.Scheduler || fs.Workers {
		t.Fatalf("expected scheduler=false, workers=false in ui mode, got scheduler=%v workers=%v", fs.Scheduler, fs.Workers)
	}
	m.Stop(t.Context())
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
	assert.MustEventually(t, func() bool {
		select {
		case <-m.Initialized():
			return true
		default:
			return false
		}
	}, 3*time.Second, 50*time.Millisecond, "wait for manager initialization")
	fs := m.FeaturesStatus()
	if !fs.Scheduler || !fs.Workers {
		t.Fatalf("expected scheduler=true, workers=true in full mode, got scheduler=%v workers=%v", fs.Scheduler, fs.Workers)
	}
	m.Stop(t.Context())
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("manager did not stop in time")
	}
}
