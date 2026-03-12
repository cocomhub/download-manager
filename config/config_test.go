// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRuntimeModeUI_DefaultEnablesTrue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	yml := []byte("runtime:\n  mode: ui\n")
	if err := os.WriteFile(path, yml, 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Runtime.Mode != RunModeUI {
		t.Fatalf("runtime.mode = %q, want %q", cfg.Runtime.Mode, RunModeUI)
	}
	if !cfg.Runtime.Download.Enabled {
		t.Fatalf("runtime.download.enabled = false, want true")
	}
	if !cfg.Runtime.Scheduler.Enabled {
		t.Fatalf("runtime.scheduler.enabled = false, want true")
	}
}

func TestLoadNoRuntime_DefaultsApplied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	// empty config
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Runtime.Mode != RunModeFull {
		t.Fatalf("runtime.mode = %q, want %q", cfg.Runtime.Mode, RunModeFull)
	}
	if !cfg.Runtime.Download.Enabled {
		t.Fatalf("runtime.download.enabled = false, want true")
	}
	if !cfg.Runtime.Scheduler.Enabled {
		t.Fatalf("runtime.scheduler.enabled = false, want true")
	}
}
