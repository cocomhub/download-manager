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
	yml := []byte("contexts:\n" +
		"  myctx:\n" +
		"    storage:\n" +
		"      type: mongo\n" +
		"      config:\n" +
		"        collection: test\n" +
		"        database: dm\n" +
		"        source: src\n" +
		"tasks:\n" +
		"  - id: t1\n" +
		"    type: tktube\n" +
		"    save_dir: /tmp\n" +
		"    storage_context: myctx\n" +
		"    extra: {}\n")
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
	yml := []byte("tasks:\n" +
		"  - id: t1\n" +
		"    type: url_list\n" +
		"    save_dir: /tmp\n" +
		"    storage:\n" +
		"      type: file\n" +
		"      config:\n" +
		"        path: /tmp/status.json\n" +
		"    extra:\n" +
		"      max_concurrent: 2\n")
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

func TestConfig_Clone(t *testing.T) {
	// Set up a Config with all map/slice fields populated
	cfg := &Config{
		Server: Server{
			HTTPPort:   8080,
			WorkDir:    "/tmp/work",
			Auth:       AuthConfig{Type: "token", Token: "secret"},
			UIDefaults: UIDefaults{DefaultSaveDir: "/saves", WindowWidth: 1024, WindowHeight: 768},
		},
		Mongo: []MongoSource{{Name: "main", URI: "mongodb://localhost"}},
		Downloader: Downloader{
			Type:             "native",
			GlobalConcurrent: 5,
			Proxies:          []string{"http://proxy1", "http://proxy2"},
			DomainLimits:     map[string]int{"example.com": 2, "test.org": 3},
			FfmpegPath:       "/usr/bin/ffmpeg",
			Filesystem:       DcFilesystem{RootDir: "/data", LogDir: "/logs", CacheDir: "/cache"},
			HTTP:             DcHTTP{TimeoutSeconds: 600},
			Proxy:            DcProxy{List: []string{"http://proxy1"}, Force: true, BandwidthPathSuffix: "/bw"},
			Progress:         DcProgress{MinPercentStep: 0.5, MaxIntervalSeconds: 10},
			FFmpeg:           DcFFmpeg{Path: "/usr/bin/ffmpeg", ExtraArgs: []string{"-v", "debug"}, HLSAutoMarkAsFail: true},
		},
		Contexts: map[string]Context{
			"pool1": {
				Storage: StorageConfig{
					Type:   "mongo",
					Config: map[string]string{"collection": "items", "database": "dm"},
				},
			},
		},
		Tasks: []Task{
			{
				ID:      "task1",
				Type:    "tktube",
				SaveDir: "/saves/tktube",
				Storage: StorageConfig{
					Type:   "file",
					Config: map[string]string{"path": "/tmp/status.json"},
				},
				Extra: map[string]any{"max_concurrent": 3, "quality": "1080p"},
			},
		},
	}

	clone := cfg.Clone()
	if clone == nil {
		t.Fatal("Clone returned nil")
	}

	// Verify equality of scalar fields
	if clone.Server.HTTPPort != cfg.Server.HTTPPort {
		t.Errorf("HTTPPort = %d, want %d", clone.Server.HTTPPort, cfg.Server.HTTPPort)
	}
	if clone.Downloader.Type != cfg.Downloader.Type {
		t.Errorf("Downloader.Type = %q, want %q", clone.Downloader.Type, cfg.Downloader.Type)
	}
	if len(clone.Tasks) != len(cfg.Tasks) {
		t.Fatalf("len(Tasks) = %d, want %d", len(clone.Tasks), len(cfg.Tasks))
	}

	// ---- Mutate the original and verify clone is untouched ----

	// Modify slice: Proxies
	cfg.Downloader.Proxies[0] = "http://evil"
	if clone.Downloader.Proxies[0] != "http://proxy1" {
		t.Errorf("clone Proxies[0] mutated to %q", clone.Downloader.Proxies[0])
	}

	// Append to slice: Proxies
	cfg.Downloader.Proxies = append(cfg.Downloader.Proxies, "http://evil2")
	if len(clone.Downloader.Proxies) != 2 {
		t.Errorf("clone Proxies length = %d, want 2", len(clone.Downloader.Proxies))
	}

	// Modify map: DomainLimits
	cfg.Downloader.DomainLimits["new"] = 99
	if _, exists := clone.Downloader.DomainLimits["new"]; exists {
		t.Error("clone DomainLimits got new key from original")
	}

	// Modify Task Extra map
	cfg.Tasks[0].Extra["new_key"] = "new_value"
	if _, exists := clone.Tasks[0].Extra["new_key"]; exists {
		t.Error("clone Tasks[0].Extra got new key from original")
	}

	// Modify Task Storage.Config map
	cfg.Tasks[0].Storage.Config["path"] = "/evil/path"
	if clone.Tasks[0].Storage.Config["path"] != "/tmp/status.json" {
		t.Errorf("clone Storage.Config['path'] mutated to %q", clone.Tasks[0].Storage.Config["path"])
	}

	// Modify Context storage config
	cfg.Contexts["pool1"].Storage.Config["collection"] = "hacked"
	if clone.Contexts["pool1"].Storage.Config["collection"] != "items" {
		t.Errorf("clone Contexts['pool1'].Storage.Config['collection'] mutated to %q",
			clone.Contexts["pool1"].Storage.Config["collection"])
	}

	// Modify Mongo slice
	cfg.Mongo[0].URI = "mongodb://evil"
	if clone.Mongo[0].URI != "mongodb://localhost" {
		t.Errorf("clone Mongo[0].URI mutated to %q", clone.Mongo[0].URI)
	}

	// Modify Proxy.List
	cfg.Downloader.Proxy.List[0] = "http://evil"
	if clone.Downloader.Proxy.List[0] != "http://proxy1" {
		t.Errorf("clone Proxy.List[0] mutated to %q", clone.Downloader.Proxy.List[0])
	}

	// Modify FFmpeg.ExtraArgs
	cfg.Downloader.FFmpeg.ExtraArgs[0] = "-evil"
	if clone.Downloader.FFmpeg.ExtraArgs[0] != "-v" {
		t.Errorf("clone FFmpeg.ExtraArgs[0] mutated to %q", clone.Downloader.FFmpeg.ExtraArgs[0])
	}

	// Test Clone on nil Config
	if (*Config)(nil).Clone() != nil {
		t.Error("nil Config.Clone() should return nil")
	}
}

func TestConfig_DefaultHTTPPort(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	cfg.ValidateAndClamp()
	if cfg.Server.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d, want 8080", cfg.Server.HTTPPort)
	}
	if cfg.Server.UIOnlyPort != 8091 {
		t.Errorf("UIOnlyPort = %d, want 8091", cfg.Server.UIOnlyPort)
	}
}

func TestConfig_FileRoot(t *testing.T) {
	t.Parallel()

	t.Run("default to filesystem root dir", func(t *testing.T) {
		cfg := &Config{}
		cfg.Downloader.Filesystem.RootDir = "/data/downloads"
		if got := cfg.FileRoot(); got != "/data/downloads" {
			t.Errorf("FileRoot() = %q, want %q", got, "/data/downloads")
		}
	})

	t.Run("use files_dir when set", func(t *testing.T) {
		cfg := &Config{}
		cfg.Server.FilesDir = "/data/files"
		cfg.Downloader.Filesystem.RootDir = "/data/downloads"
		if got := cfg.FileRoot(); got != "/data/files" {
			t.Errorf("FileRoot() = %q, want %q", got, "/data/files")
		}
	})

	t.Run("use download_root_dir when set", func(t *testing.T) {
		cfg := &Config{}
		cfg.Server.DownloadRootDir = "/data/downloads"
		cfg.Downloader.Filesystem.RootDir = "/data/fallback"
		if got := cfg.FileRoot(); got != "/data/downloads" {
			t.Errorf("FileRoot() = %q, want %q", got, "/data/downloads")
		}
	})

	t.Run("all empty returns empty", func(t *testing.T) {
		cfg := &Config{}
		if got := cfg.FileRoot(); got != "" {
			t.Errorf("FileRoot() = %q, want empty", got)
		}
	})
}

// === TaskTypeDefaults ===

func TestTaskTypeDefaults_DefaultValues(t *testing.T) {
	// 空 Config → ValidateAndClamp 后已知类型默认全 true
	cfg := &Config{Server: Server{WorkDir: t.TempDir()}}
	cfg.ValidateAndClamp()
	for _, typ := range []string{"hanime", "tktube", "vikacg", "urllist"} {
		def, ok := cfg.TaskTypeDefaults[typ]
		if !ok {
			t.Fatalf("missing default for %q", typ)
		}
		if def.ScrapeEnabled == nil || !*def.ScrapeEnabled {
			t.Errorf("%s.ScrapeEnabled = %v, want true", typ, def.ScrapeEnabled)
		}
		if def.DownloadEnabled == nil || !*def.DownloadEnabled {
			t.Errorf("%s.DownloadEnabled = %v, want true", typ, def.DownloadEnabled)
		}
	}
}

func TestTaskTypeDefaults_Override(t *testing.T) {
	// 任务级覆盖类型默认值
	cfg := &Config{Server: Server{WorkDir: t.TempDir()}}
	cfg.TaskTypeDefaults = map[string]TaskTypeDefault{
		"hanime": {ScrapeEnabled: new(true), DownloadEnabled: new(true)},
	}
	cfg.Tasks = []Task{
		{ID: "t1", Type: "hanime", ScrapeEnabled: new(false)},
		{ID: "t2", Type: "hanime"}, // 使用类型默认值
	}
	cfg.ValidateAndClamp()
	if cfg.Tasks[0].GetScrapeEnabled(cfg) != false {
		t.Error("t1: GetScrapeEnabled should be false (override)")
	}
	if cfg.Tasks[0].GetDownloadEnabled(cfg) != true {
		t.Error("t1: GetDownloadEnabled should be true (inherited)")
	}
	if cfg.Tasks[1].GetScrapeEnabled(cfg) != true {
		t.Error("t2: GetScrapeEnabled should be true (inherited)")
	}
}

func TestTaskTypeDefaults_GetEffectiveSaveDir(t *testing.T) {
	cfg := &Config{Server: Server{WorkDir: t.TempDir()}}
	cfg.TaskTypeDefaults = map[string]TaskTypeDefault{
		"hanime": {SaveRootDir: "/data"},
	}
	t.Run("save_dir takes precedence", func(t *testing.T) {
		task := Task{ID: "t1", Type: "hanime", SaveDir: "/custom", SaveSubDir: "sub"}
		got := task.GetEffectiveSaveDir(cfg)
		if got != "/custom" {
			t.Fatalf("got %q, want /custom", got)
		}
	})
	t.Run("save_root_dir + save_sub_dir", func(t *testing.T) {
		task := Task{ID: "t2", Type: "hanime", SaveSubDir: "videos"}
		got := task.GetEffectiveSaveDir(cfg)
		want := filepath.Join("/data", "videos")
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
	t.Run("only save_root_dir", func(t *testing.T) {
		task := Task{ID: "t3", Type: "hanime"}
		got := task.GetEffectiveSaveDir(cfg)
		if got != "/data" {
			t.Fatalf("got %q, want /data", got)
		}
	})
	t.Run("no default", func(t *testing.T) {
		task := Task{ID: "t4", Type: "unknown"}
		got := task.GetEffectiveSaveDir(cfg)
		if got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
}

func TestTaskTypeDefaults_SanitizeForSave(t *testing.T) {
	// 与默认值匹配的字段应被移除
	cfg := &Config{Server: Server{WorkDir: t.TempDir()}}
	trueVal := true
	cfg.TaskTypeDefaults = map[string]TaskTypeDefault{
		"hanime": {ScrapeEnabled: &trueVal, DownloadEnabled: &trueVal},
	}
	cfg.Tasks = []Task{
		{ID: "t1", Type: "hanime", ScrapeEnabled: &trueVal, DownloadEnabled: &trueVal}, // 全匹配默认 → 应移除
		{ID: "t2", Type: "hanime", ScrapeEnabled: new(false)},                          // 不匹配 → 保留
	}
	cleaned := cfg.SanitizeForSave()
	if cleaned.Tasks[0].ScrapeEnabled != nil {
		t.Error("t1: ScrapeEnabled should be nil (sanitized)")
	}
	if cleaned.Tasks[0].DownloadEnabled != nil {
		t.Error("t1: DownloadEnabled should be nil (sanitized)")
	}
	if cleaned.Tasks[1].ScrapeEnabled == nil || *cleaned.Tasks[1].ScrapeEnabled != false {
		t.Error("t2: ScrapeEnabled should be false (preserved)")
	}
}

func TestTaskTypeDefaults_Clone(t *testing.T) {
	// 验证 TaskTypeDefaults 深拷贝
	trueVal := true
	cfg := &Config{Server: Server{WorkDir: t.TempDir()}}
	cfg.TaskTypeDefaults = map[string]TaskTypeDefault{
		"hanime": {ScrapeEnabled: &trueVal, DownloadEnabled: &trueVal, SaveRootDir: "/data"},
	}
	clone := cfg.Clone()
	// 修改原始值
	cfg.TaskTypeDefaults["hanime"] = TaskTypeDefault{SaveRootDir: "/evil"}
	if clone.TaskTypeDefaults["hanime"].SaveRootDir != "/data" {
		t.Error("clone should not be affected by original mutation")
	}
}

func TestTaskTypeDefaults_Diff(t *testing.T) {
	a := Config{TaskTypeDefaults: map[string]TaskTypeDefault{"hanime": {SaveRootDir: "/a"}}}
	b := Config{TaskTypeDefaults: map[string]TaskTypeDefault{"hanime": {SaveRootDir: "/b"}}}
	changes := a.Diff(b)
	found := false
	for _, c := range changes {
		if c.Path == "task_type_defaults" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected task_type_defaults change in diff")
	}
}

func TestTaskTypeDefaults_BackwardCompat(t *testing.T) {
	// 旧 config.yaml 没有 task_type_defaults → 加载后 ValidateAndClamp 填充默认值
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	yml := []byte("tasks:\n  - id: t1\n    type: hanime\n    save_dir: /tmp\n")
	if err := os.WriteFile(path, yml, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(cfg.TaskTypeDefaults) == 0 {
		t.Error("expected TaskTypeDefaults to be populated")
	}
	if !cfg.Tasks[0].GetScrapeEnabled(cfg) {
		t.Error("t1: GetScrapeEnabled should default to true")
	}
	if !cfg.Tasks[0].GetDownloadEnabled(cfg) {
		t.Error("t1: GetDownloadEnabled should default to true")
	}
}

func TestTaskTypeDefaults_StorageDefaults(t *testing.T) {
	// 测试类型默认值中 storage 配置被正确保留
	cfg := &Config{Server: Server{WorkDir: t.TempDir()}}
	cfg.TaskTypeDefaults = map[string]TaskTypeDefault{
		"hanime": {Storage: &StorageConfig{Type: "file", Config: map[string]string{"path": "/data"}}},
	}
	cfg.ValidateAndClamp()
	def, ok := cfg.TaskTypeDefaults["hanime"]
	if !ok {
		t.Fatal("missing default for hanime")
	}
	if def.Storage == nil {
		t.Fatal("Storage should not be nil")
	}
	if def.Storage.Type != "file" {
		t.Errorf("Storage.Type = %q, want %q", def.Storage.Type, "file")
	}
	if def.Storage.Config["path"] != "/data" {
		t.Errorf("Storage.Config[path] = %q, want %q", def.Storage.Config["path"], "/data")
	}
}
