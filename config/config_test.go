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

func TestValidateAndClamp_ResolvesContextReference(t *testing.T) {
	cfg := &Config{
		Contexts: map[string]Context{
			"tktube_pool": {
				Storage: StorageConfig{
					Type: "mongo",
					Config: map[string]string{
						"collection": "tktube",
						"database":   "download_manager",
						"source":     "suixi-mini",
					},
				},
			},
		},
		Tasks: []Task{
			{
				ID:             "test_task",
				StorageContext: "tktube_pool",
				// Storage is zero-value (empty), should be resolved from context
			},
		},
	}
	cfg.ValidateAndClamp()

	if cfg.Tasks[0].Storage.Type != "mongo" {
		t.Fatalf("Storage.Type = %q, want %q", cfg.Tasks[0].Storage.Type, "mongo")
	}
	if cfg.Tasks[0].Storage.Config["collection"] != "tktube" {
		t.Fatalf("Storage.Config[collection] = %q, want %q", cfg.Tasks[0].Storage.Config["collection"], "tktube")
	}
	if cfg.Tasks[0].StorageContext != "tktube_pool" {
		t.Fatalf("StorageContext = %q, want %q", cfg.Tasks[0].StorageContext, "tktube_pool")
	}
}

func TestValidateAndClamp_InlineStorageOverridesContext(t *testing.T) {
	cfg := &Config{
		Contexts: map[string]Context{
			"default": {Storage: StorageConfig{Type: "mongo", Config: map[string]string{"db": "a"}}},
		},
		Tasks: []Task{
			{
				ID:             "test_task",
				StorageContext: "default",
				Storage: StorageConfig{
					Type: "file",
					Config: map[string]string{
						"path": "/tmp/status.json",
					},
				},
			},
		},
	}
	cfg.ValidateAndClamp()

	// Inline "file" storage should be preserved, NOT overridden by context's "mongo"
	if cfg.Tasks[0].Storage.Type != "file" {
		t.Fatalf("Storage.Type = %q, want %q", cfg.Tasks[0].Storage.Type, "file")
	}
	if cfg.Tasks[0].Storage.Config["path"] != "/tmp/status.json" {
		t.Fatalf("Storage.Config[path] = %q, want %q", cfg.Tasks[0].Storage.Config["path"], "/tmp/status.json")
	}
}

func TestValidateAndClamp_MissingContextDoesNotPanic(t *testing.T) {
	// This test verifies that referencing a non-existent context does not panic
	// and leaves Storage as zero-value (Type == "").
	cfg := &Config{
		Tasks: []Task{
			{ID: "orphan", StorageContext: "nonexistent"},
		},
	}
	cfg.ValidateAndClamp()
	if cfg.Tasks[0].Storage.Type != "" {
		t.Fatalf("expected empty Storage.Type for unresolved context, got %q", cfg.Tasks[0].Storage.Type)
	}
}

func TestValidateAndClamp_NoContextsIsNoOp(t *testing.T) {
	// Backward compatibility: existing config with inline storage but no contexts block
	// must work identically — Storage should remain unchanged.
	cfg := &Config{
		Tasks: []Task{
			{
				ID: "legacy",
				Storage: StorageConfig{
					Type: "mongo",
					Config: map[string]string{
						"collection": "test",
						"database":   "dm",
						"source":     "src",
					},
				},
			},
		},
	}
	cfg.ValidateAndClamp()

	if cfg.Tasks[0].Storage.Type != "mongo" {
		t.Fatalf("Storage.Type = %q, want %q", cfg.Tasks[0].Storage.Type, "mongo")
	}
	if cfg.Tasks[0].StorageContext != "" {
		t.Fatalf("StorageContext should be empty when not set, got %q", cfg.Tasks[0].StorageContext)
	}
}

func TestValidateAndClamp_EmptyContextsMap(t *testing.T) {
	// Contexts map is non-nil but empty, references should log warning, not panic.
	cfg := &Config{
		Contexts: map[string]Context{},
		Tasks: []Task{
			{ID: "orphan", StorageContext: "missing"},
		},
	}
	cfg.ValidateAndClamp()
	if cfg.Tasks[0].Storage.Type != "" {
		t.Fatalf("expected empty Storage.Type for unresolved context, got %q", cfg.Tasks[0].Storage.Type)
	}
}

func TestLoadConfigWithContexts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	yml := []byte(`
contexts:
  myctx:
    storage:
      type: mongo
      config:
        collection: test
        database: dm
        source: src
tasks:
  - id: t1
    type: tktube
    save_dir: /tmp
    storage_context: myctx
    extra: {}
`)
	if err := os.WriteFile(path, yml, 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Tasks[0].Storage.Type != "mongo" {
		t.Fatalf("Storage.Type = %q, want %q", cfg.Tasks[0].Storage.Type, "mongo")
	}
	if cfg.Tasks[0].StorageContext != "myctx" {
		t.Fatalf("StorageContext = %q, want %q", cfg.Tasks[0].StorageContext, "myctx")
	}
}

func TestLoadConfigWithoutContexts_BackwardCompatible(t *testing.T) {
	// An existing config with inline storage and no contexts section must
	// be loaded identically before and after the feature addition.
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	yml := []byte(`
tasks:
  - id: t1
    type: url_list
    save_dir: /tmp
    storage:
      type: file
      config:
        path: /tmp/status.json
    extra:
      max_concurrent: 2
`)
	if err := os.WriteFile(path, yml, 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Tasks[0].Storage.Type != "file" {
		t.Fatalf("Storage.Type = %q, want %q", cfg.Tasks[0].Storage.Type, "file")
	}
	if cfg.Tasks[0].Storage.Config["path"] != "/tmp/status.json" {
		t.Fatalf("Storage.Config[path] = %q, want %q", cfg.Tasks[0].Storage.Config["path"], "/tmp/status.json")
	}
	if cfg.Tasks[0].StorageContext != "" {
		t.Fatalf("StorageContext should be empty, got %q", cfg.Tasks[0].StorageContext)
	}
}

func TestDiff_ContextChanges(t *testing.T) {
	a := Config{
		Contexts: map[string]Context{
			"ctx1": {Storage: StorageConfig{Type: "mongo"}},
		},
	}
	b := Config{
		Contexts: map[string]Context{
			"ctx2": {Storage: StorageConfig{Type: "file"}},
		},
	}
	changes := a.Diff(b)
	found := false
	for _, c := range changes {
		if c.Path == "contexts" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'contexts' change in diff")
	}
}

func TestDiff_StorageContextChanges(t *testing.T) {
	a := Config{
		Tasks: []Task{
			{ID: "t1", StorageContext: "ctx_a"},
		},
	}
	b := Config{
		Tasks: []Task{
			{ID: "t1", StorageContext: "ctx_b"},
		},
	}
	changes := a.Diff(b)
	found := false
	for _, c := range changes {
		if c.Path == "tasks.t1.storage_context" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'tasks.t1.storage_context' change in diff")
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
