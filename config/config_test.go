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

func TestValidateAndClampDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Server: Server{
			WorkDir: dir,
		},
	}
	cfg.ValidateAndClamp()

	if got, want := cfg.Downloader.Filesystem.RootDir, filepath.Join(dir, "downloads"); got != want {
		t.Fatalf("Filesystem.RootDir = %q, want %q", got, want)
	}
	if cfg.Downloader.HTTP.TimeoutSeconds == 0 ||
		cfg.Downloader.HTTP.IdleConnTimeoutSeconds == 0 ||
		cfg.Downloader.HTTP.MaxIdleConns == 0 ||
		cfg.Downloader.HTTP.MaxIdleConnsPerHost == 0 {
		t.Fatalf("HTTP defaults should be non-zero, got: %+v", cfg.Downloader.HTTP)
	}
	if got, want := cfg.Downloader.Filesystem.LogDir, "logs"; got != want {
		t.Fatalf("Filesystem.LogDir = %q, want %q", got, want)
	}
	if got, want := cfg.Downloader.Filesystem.CacheDir, ".cache"; got != want {
		t.Fatalf("Filesystem.CacheDir = %q, want %q", got, want)
	}
	if got, want := cfg.Downloader.Proxy.DecisionCacheTTLSecs, 1; got != want {
		t.Fatalf("Proxy.DecisionCacheTTLSecs = %d, want %d", got, want)
	}
	if got, want := cfg.Downloader.Proxy.DirectProbeTimeoutSecs, 3; got != want {
		t.Fatalf("Proxy.DirectProbeTimeoutSecs = %d, want %d", got, want)
	}
	if got, want := cfg.Downloader.Proxy.BandwidthPathSuffix, "/bandwidth"; got != want {
		t.Fatalf("Proxy.BandwidthPathSuffix = %q, want %q", got, want)
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
