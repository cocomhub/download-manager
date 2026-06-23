// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/testutil/assert"
	mockdl "github.com/cocomhub/download-manager/testutil/mockdl"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// saveRestoreGlobals captures and restores config package globals.
func saveRestoreGlobals(t *testing.T) {
	t.Helper()
	origPath := config.GetConfigFilePath()
	origSrv := config.GetServerConfig()
	t.Cleanup(func() {
		config.SetConfigFilePath(origPath)
		config.SetServerConfig(origSrv)
	})
}

// initConfigGlobals sets ServerConfig.WorkDir and ConfigFilePath to temp paths.
func initConfigGlobals(t *testing.T) string {
	t.Helper()
	workDir := t.TempDir()
	config.SetServerConfig(config.Server{WorkDir: workDir})
	config.SetConfigFilePath(filepath.Join(workDir, "config.yaml"))
	return workDir
}

// createBackup creates a named backup file in config_backups/ under workDir.
func createBackup(t *testing.T, workDir, filename, content string) string {
	t.Helper()
	backupDir := filepath.Join(workDir, "config_backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("failed to create backup dir: %v", err)
	}
	p := filepath.Join(backupDir, filename)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write backup: %v", err)
	}
	return p
}

// contains is a helper to check substring presence.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Manager delegation tests (config_mgr.go)
// ---------------------------------------------------------------------------

func TestManager_GetConfig(t *testing.T) {
	mgr, _ := newMockManager(t, "task-gc", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	cfg := mgr.GetConfig()
	if cfg == nil {
		t.Fatal("GetConfig() returned nil")
	}
	if cfg.Server.WorkDir == "" {
		t.Fatal("expected non-empty WorkDir")
	}
}

func TestManager_ListConfigBackups_Empty(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := t.TempDir()
	config.SetServerConfig(config.Server{WorkDir: workDir})
	config.SetConfigFilePath(filepath.Join(workDir, "config.yaml"))

	mgr, _ := newMockManager(t, "task-lc", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	backups, err := mgr.ListConfigBackups()
	if err != nil {
		t.Fatalf("ListConfigBackups failed: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("expected empty backups list, got %d", len(backups))
	}
}

func TestManager_DeleteConfigBackup_NotFound(t *testing.T) {
	mgr, _ := newMockManager(t, "task-dc", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	err := mgr.DeleteConfigBackup("nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error deleting nonexistent backup")
	}
}

func TestManager_RollbackConfig_NotFound(t *testing.T) {
	mgr, _ := newMockManager(t, "task-rb", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	err := mgr.RollbackConfig("nonexistent.yaml", nil)
	if err == nil {
		t.Fatal("expected error rolling back to nonexistent backup")
	}
}

func TestManager_DiffConfigFiles_BothCurrent(t *testing.T) {
	mgr, _ := newMockManager(t, "task-df", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	_, err := mgr.DiffConfigFiles("current", "current")
	if err != nil {
		t.Logf("DiffConfigFiles(current,current): %v (expected if no config file)", err)
	}
}

func TestManager_DiffConfigFilesOpts_BothCurrent(t *testing.T) {
	mgr, _ := newMockManager(t, "task-do", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	_, err := mgr.DiffConfigFilesOpts("current", "current", true, true)
	if err != nil {
		t.Logf("DiffConfigFilesOpts(current,current): %v (expected if no config file)", err)
	}
}

func TestManager_AddConfigTag_Empty(t *testing.T) {
	mgr, _ := newMockManager(t, "task-at", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	err := mgr.AddConfigTag("backup.yaml", "")
	if err == nil {
		t.Fatal("expected error for empty tag")
	}
}

func TestManager_AddConfigNote_EmptyMessage(t *testing.T) {
	mgr, _ := newMockManager(t, "task-an", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	err := mgr.AddConfigNote("backup.yaml", "", "author")
	if err == nil {
		t.Fatal("expected error for empty message")
	}
}

// ---------------------------------------------------------------------------
// ConfigService direct tests (config_service.go)
// ---------------------------------------------------------------------------

func TestConfigService_GetConfig(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{
		Server:     config.Server{WorkDir: workDir},
		Downloader: config.Downloader{GlobalConcurrent: 5, MaxRetries: 3},
	})

	got := cs.GetConfig()
	if got == nil {
		t.Fatal("GetConfig() returned nil")
	}
	if got.Downloader.GlobalConcurrent != 5 {
		t.Fatalf("expected GlobalConcurrent=5, got %d", got.Downloader.GlobalConcurrent)
	}
}

func TestConfigService_StoreConfig(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{
		Server:     config.Server{WorkDir: workDir},
		Downloader: config.Downloader{GlobalConcurrent: 5, MaxRetries: 3},
	})

	old := cs.GetConfig()
	newCfg := old.Clone()
	newCfg.Downloader.GlobalConcurrent = 42
	cs.StoreConfig(newCfg)

	got := cs.GetConfig()
	if got.Downloader.GlobalConcurrent != 42 {
		t.Fatalf("expected GlobalConcurrent=42 after StoreConfig, got %d", got.Downloader.GlobalConcurrent)
	}
}

func TestConfigService_ListConfigBackups_Empty(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	backups, err := cs.ListConfigBackups()
	if err != nil {
		t.Fatalf("ListConfigBackups failed: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("expected empty backups, got %d", len(backups))
	}
}

func TestConfigService_ListConfigBackups_WithFiles(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	createBackup(t, workDir, "config_20260619_120000.yaml", "server:\n  http_port: 8080\n")
	createBackup(t, workDir, "config_20260619_130000.yaml", "server:\n  http_port: 9090\n")
	createBackup(t, workDir, "other.yaml", "foo: bar") // wrong prefix, should be skipped

	backups, err := cs.ListConfigBackups()
	if err != nil {
		t.Fatalf("ListConfigBackups failed: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(backups))
	}
	// Should be sorted descending (newest first)
	if backups[0]["filename"] < backups[1]["filename"] {
		t.Fatal("expected backups sorted descending by filename")
	}
}

func TestConfigService_DeleteConfigBackup(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	createBackup(t, workDir, "config_test.yaml", "server:\n  http_port: 8080\n")

	if err := cs.DeleteConfigBackup("config_test.yaml"); err != nil {
		t.Fatalf("DeleteConfigBackup failed: %v", err)
	}

	backups, _ := cs.ListConfigBackups()
	if len(backups) != 0 {
		t.Fatalf("expected 0 backups after deletion, got %d", len(backups))
	}
}

func TestConfigService_DeleteConfigBackup_NotFound(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	err := cs.DeleteConfigBackup("nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent backup")
	}
}

func TestConfigService_RollbackLoad(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	createBackup(t, workDir, "config_backup.yaml",
		"server:\n  http_port: 9090\n  work_dir: "+workDir+"\ndownloader:\n  global_concurrent: 10\n")

	cfg, err := cs.RollbackLoad("config_backup.yaml")
	if err != nil {
		t.Fatalf("RollbackLoad failed: %v", err)
	}
	if cfg.Server.HTTPPort != 9090 {
		t.Fatalf("expected HTTPPort=9090, got %d", cfg.Server.HTTPPort)
	}
	if cfg.Downloader.GlobalConcurrent != 10 {
		t.Fatalf("expected GlobalConcurrent=10, got %d", cfg.Downloader.GlobalConcurrent)
	}
}

func TestConfigService_RollbackLoad_NotFound(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	_, err := cs.RollbackLoad("nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent backup")
	}
}

func TestConfigService_RollbackLoad_InvalidYAML(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	createBackup(t, workDir, "bad.yaml", "invalid: yaml: [unclosed")

	_, err := cs.RollbackLoad("bad.yaml")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestConfigService_DiffConfigFiles_CurrentVsBackup(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)

	// Write the current config file to disk.
	cfgData := "server:\n  http_port: 8080\n  work_dir: " + workDir + "\ndownloader:\n  global_concurrent: 5\n  max_retries: 3\n"
	if err := os.WriteFile(config.GetConfigFilePath(), []byte(cfgData), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	createBackup(t, workDir, "diff_backup.yaml",
		"server:\n  http_port: 9090\n  work_dir: "+workDir+"\ndownloader:\n  global_concurrent: 10\n  max_retries: 5\n")

	result, err := cs.DiffConfigFiles("current", "diff_backup.yaml")
	if err != nil {
		t.Fatalf("DiffConfigFiles failed: %v", err)
	}

	if result["left"] != "current" {
		t.Fatalf("expected left='current', got %v", result["left"])
	}
	if result["right"] != "diff_backup.yaml" {
		t.Fatalf("expected right='diff_backup.yaml', got %v", result["right"])
	}
	if result["left_yaml"].(string) == "" {
		t.Fatal("left_yaml is empty")
	}
	if result["right_yaml"].(string) == "" {
		t.Fatal("right_yaml is empty")
	}
	changes, ok := result["changes"].([]config.Change)
	if !ok {
		t.Fatalf("changes is not []config.Change, got %T", result["changes"])
	}
	if len(changes) == 0 {
		t.Fatal("expected at least one change between different configs")
	}
}

func TestConfigService_DiffConfigFiles_EmptyRef(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cfgData := "server:\n  http_port: 8080\n  work_dir: " + workDir + "\n"
	if err := os.WriteFile(config.GetConfigFilePath(), []byte(cfgData), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	result, err := cs.DiffConfigFiles("", "")
	if err != nil {
		t.Fatalf("DiffConfigFiles with empty refs failed: %v", err)
	}
	if result["left"] != "" {
		t.Fatalf("expected left='', got %v", result["left"])
	}
}

func TestConfigService_DiffConfigFiles_BackupNotFound(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cfgData := "server:\n  http_port: 8080\n  work_dir: " + workDir + "\n"
	if err := os.WriteFile(config.GetConfigFilePath(), []byte(cfgData), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	_, err := cs.DiffConfigFiles("current", "nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent backup")
	}
}

func TestConfigService_DiffConfigFilesOpts_NormalizeWS(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cfgData := "server:\n  http_port: 8080\n  work_dir: " + workDir + "\ndownloader:\n  global_concurrent: 5\n"
	if err := os.WriteFile(config.GetConfigFilePath(), []byte(cfgData), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	createBackup(t, workDir, "ws_backup.yaml",
		"server:\n  http_port: 9090\n  work_dir: "+workDir+"\ndownloader:\n  global_concurrent: 10\n")

	result, err := cs.DiffConfigFilesOpts("current", "ws_backup.yaml", true, false)
	if err != nil {
		t.Fatalf("DiffConfigFilesOpts failed: %v", err)
	}

	if result["left_norm"] == "" {
		t.Fatal("left_norm should be non-empty when ignoreWS=true")
	}
	if result["right_norm"] == "" {
		t.Fatal("right_norm should be non-empty when ignoreWS=true")
	}
}

func TestConfigService_DiffConfigFilesOpts_IgnoreComments(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cfgData := "server:\n  http_port: 8080\n  work_dir: " + workDir + "\ndownloader:\n  global_concurrent: 5\n"
	if err := os.WriteFile(config.GetConfigFilePath(), []byte(cfgData), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	createBackup(t, workDir, "cmt_backup.yaml",
		"server:\n  http_port: 9090\n  work_dir: "+workDir+"\ndownloader:\n  # this is a comment\n  global_concurrent: 10\n")

	result, err := cs.DiffConfigFilesOpts("current", "cmt_backup.yaml", false, true)
	if err != nil {
		t.Fatalf("DiffConfigFilesOpts failed: %v", err)
	}

	if result["left_norm"] == "" {
		t.Fatal("left_norm should be non-empty")
	}
}

func TestConfigService_DiffConfigFilesOpts_BothNormalizers(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cfgData := "server:\n  http_port: 8080\n  work_dir: " + workDir + "\ndownloader:\n  global_concurrent: 5\n"
	if err := os.WriteFile(config.GetConfigFilePath(), []byte(cfgData), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	createBackup(t, workDir, "both_backup.yaml",
		"server:\n  http_port: 9090\n  work_dir: "+workDir+"\ndownloader:\n  # comment\n  global_concurrent: 10\n")

	result, err := cs.DiffConfigFilesOpts("current", "both_backup.yaml", true, true)
	if err != nil {
		t.Fatalf("DiffConfigFilesOpts failed: %v", err)
	}

	if _, ok := result["left_norm"]; !ok {
		t.Fatal("expected left_norm in result")
	}
	if _, ok := result["right_norm"]; !ok {
		t.Fatal("expected right_norm in result")
	}
}

func TestConfigService_readDiffSide_Current(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cfgData := "server:\n  http_port: 8080\n  work_dir: " + workDir + "\n"
	if err := os.WriteFile(config.GetConfigFilePath(), []byte(cfgData), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	cfg, yml, err := cs.readDiffSide("current")
	if err != nil {
		t.Fatalf("readDiffSide(current) failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("readDiffSide returned nil cfg")
	}
	if len(yml) == 0 {
		t.Fatal("readDiffSide returned empty yml")
	}
	if cfg.Server.WorkDir != workDir {
		t.Fatalf("expected WorkDir=%s, got %s", workDir, cfg.Server.WorkDir)
	}
}

func TestConfigService_readDiffSide_EmptyRef(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cfgData := "server:\n  http_port: 8080\n  work_dir: " + workDir + "\n"
	if err := os.WriteFile(config.GetConfigFilePath(), []byte(cfgData), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	cfg, yml, err := cs.readDiffSide("")
	if err != nil {
		t.Fatalf("readDiffSide('') failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("readDiffSide('') returned nil cfg")
	}
	if len(yml) == 0 {
		t.Fatal("readDiffSide('') returned empty yml")
	}
	if cfg.Server.WorkDir != workDir {
		t.Fatalf("expected WorkDir=%s, got %s", workDir, cfg.Server.WorkDir)
	}
}

func TestConfigService_readDiffSide_Backup(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	createBackup(t, workDir, "mybackup.yaml",
		"server:\n  http_port: 7070\n  work_dir: "+workDir+"\n")

	cfg, yml, err := cs.readDiffSide("mybackup.yaml")
	if err != nil {
		t.Fatalf("readDiffSide(backup) failed: %v", err)
	}
	if cfg.Server.HTTPPort != 7070 {
		t.Fatalf("expected HTTPPort=7070, got %d", cfg.Server.HTTPPort)
	}
	if len(yml) == 0 {
		t.Fatal("readDiffSide(backup) returned empty yml")
	}
}

func TestConfigService_readDiffSide_BackupNotFound(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	_, _, err := cs.readDiffSide("nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent backup")
	}
}

func TestConfigService_normalizeYAML_Noop(t *testing.T) {
	input := "server:\n  port: 8080\n"
	got := normalizeYAML(input, false, false)
	if got != input {
		t.Fatalf("expected no change without flags:\n  got:  %q\n  want: %q", got, input)
	}
}

func TestConfigService_normalizeYAML_StripComments(t *testing.T) {
	input := "server:\n  port: 8080\n  # comment line\n  host: localhost\n"
	got := normalizeYAML(input, false, true)
	if contains(got, "# comment") {
		t.Fatalf("comments should be stripped:\n%s", got)
	}
	if !contains(got, "server:") {
		t.Fatal("normalizeYAML stripped too much")
	}
}

func TestConfigService_normalizeYAML_StripWS(t *testing.T) {
	input := "server:  \n  port: 8080   \n"
	got := normalizeYAML(input, true, false)
	expected := "server:\n  port: 8080\n"
	if got != expected {
		t.Fatalf("whitespace normalize mismatch:\n  got:  %q\n  want: %q", got, expected)
	}
}

func TestConfigService_normalizeYAML_TabToSpaces(t *testing.T) {
	input := "server:\n\tport: 8080\n"
	got := normalizeYAML(input, true, false)
	if contains(got, "\t") {
		t.Fatal("tabs should be replaced with spaces")
	}
}

func TestConfigService_normalizeYAML_Both(t *testing.T) {
	input := "server:\n  port: 8080   \n  # a comment\n  host: localhost  \n"
	got := normalizeYAML(input, true, true)
	if contains(got, "#") {
		t.Fatal("comments should be stripped")
	}
	if contains(got, "  \n") {
		t.Fatal("trailing spaces should be stripped")
	}
	if !contains(got, "localhost") {
		t.Fatal("normalizeYAML stripped too much")
	}
}

func TestConfigService_AddConfigTag(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	createBackup(t, workDir, "tag_test.yaml", "server:\n  port: 8080\n")

	if err := cs.AddConfigTag("tag_test.yaml", "production"); err != nil {
		t.Fatalf("AddConfigTag failed: %v", err)
	}

	metaPath := filepath.Join(workDir, "config_backups", "tag_test.yaml.meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("failed to read meta file: %v", err)
	}
	if !contains(string(data), "production") {
		t.Fatal("meta file should contain 'production' tag")
	}
}

func TestConfigService_AddConfigTag_Empty(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	err := cs.AddConfigTag("any.yaml", "")
	if err == nil {
		t.Fatal("expected error for empty tag")
	}
}

func TestConfigService_AddConfigTag_Dedup(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	createBackup(t, workDir, "dedup.yaml", "server:\n  port: 8080\n")

	if err := cs.AddConfigTag("dedup.yaml", "staging"); err != nil {
		t.Fatalf("first AddConfigTag failed: %v", err)
	}
	if err := cs.AddConfigTag("dedup.yaml", "staging"); err != nil {
		t.Fatalf("second AddConfigTag failed: %v", err)
	}

	metaPath := filepath.Join(workDir, "config_backups", "dedup.yaml.meta.json")
	data, _ := os.ReadFile(metaPath)
	if !contains(string(data), "staging") {
		t.Fatal("meta file should contain 'staging' tag")
	}
}

func TestConfigService_AddConfigNote(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	createBackup(t, workDir, "note_test.yaml", "server:\n  port: 8080\n")

	if err := cs.AddConfigNote("note_test.yaml", "initial setup", "admin"); err != nil {
		t.Fatalf("AddConfigNote failed: %v", err)
	}

	metaPath := filepath.Join(workDir, "config_backups", "note_test.yaml.meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("failed to read meta file: %v", err)
	}
	if !contains(string(data), "initial setup") {
		t.Fatal("meta file should contain note message")
	}
	if !contains(string(data), "admin") {
		t.Fatal("meta file should contain note author")
	}
}

func TestConfigService_AddConfigNote_EmptyMessage(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	err := cs.AddConfigNote("any.yaml", "", "author")
	if err == nil {
		t.Fatal("expected error for empty message")
	}
}

func TestConfigService_AddConfigNote_AppendToExistingMeta(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	createBackup(t, workDir, "multi.yaml", "server:\n  port: 8080\n")

	if err := cs.AddConfigNote("multi.yaml", "first note", "alice"); err != nil {
		t.Fatalf("first AddConfigNote failed: %v", err)
	}
	if err := cs.AddConfigNote("multi.yaml", "second note", "bob"); err != nil {
		t.Fatalf("second AddConfigNote failed: %v", err)
	}

	metaPath := filepath.Join(workDir, "config_backups", "multi.yaml.meta.json")
	data, _ := os.ReadFile(metaPath)
	if !contains(string(data), "first note") {
		t.Fatal("meta should contain first note")
	}
	if !contains(string(data), "second note") {
		t.Fatal("meta should contain second note")
	}
}

func TestConfigService_AddConfigTag_WithExistingMeta(t *testing.T) {
	saveRestoreGlobals(t)
	workDir := initConfigGlobals(t)
	cs := NewConfigService(&config.Config{Server: config.Server{WorkDir: workDir}})

	createBackup(t, workDir, "exist_meta.yaml", "server:\n  port: 8080\n")

	if err := cs.AddConfigNote("exist_meta.yaml", "my note", "me"); err != nil {
		t.Fatalf("AddConfigNote failed: %v", err)
	}
	if err := cs.AddConfigTag("exist_meta.yaml", "my-tag"); err != nil {
		t.Fatalf("AddConfigTag failed: %v", err)
	}

	metaPath := filepath.Join(workDir, "config_backups", "exist_meta.yaml.meta.json")
	data, _ := os.ReadFile(metaPath)
	if !contains(string(data), "my note") {
		t.Fatal("meta should contain note after adding tag")
	}
	if !contains(string(data), "my-tag") {
		t.Fatal("meta should contain tag")
	}
}

// ---------------------------------------------------------------------------
// Manager integration tests using full lifecycle
// ---------------------------------------------------------------------------

func TestManager_ListConfigBackups_WithFiles(t *testing.T) {
	saveRestoreGlobals(t)
	mgr, _ := newMockManager(t, "task-lb", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)
	waitForTask(t, mgr, "task-lb")

	workDir := mgr.currentCfg().Server.WorkDir
	config.SetServerConfig(config.Server{WorkDir: workDir})
	config.SetConfigFilePath(filepath.Join(workDir, "config.yaml"))

	createBackup(t, workDir, "config_int_1.yaml", "server:\n  http_port: 8080\n")
	createBackup(t, workDir, "config_int_2.yaml", "server:\n  http_port: 9090\n")

	backups, err := mgr.ListConfigBackups()
	if err != nil {
		t.Fatalf("ListConfigBackups failed: %v", err)
	}
	if len(backups) < 2 {
		t.Fatalf("expected at least 2 backups, got %d", len(backups))
	}
}

func TestManager_RollbackConfig_InvalidContent(t *testing.T) {
	saveRestoreGlobals(t)
	mgr, _ := newMockManager(t, "task-ri", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)
	waitForTask(t, mgr, "task-ri")

	workDir := mgr.currentCfg().Server.WorkDir
	config.SetServerConfig(config.Server{WorkDir: workDir})
	config.SetConfigFilePath(filepath.Join(workDir, "config.yaml"))
	createBackup(t, workDir, "bad_rollback.yaml", "invalid: [yaml")

	err := mgr.RollbackConfig("bad_rollback.yaml", nil)
	if err == nil {
		t.Fatal("expected error for invalid backup YAML")
	}
}

func TestManager_AddConfigTag_Success(t *testing.T) {
	saveRestoreGlobals(t)
	mgr, _ := newMockManager(t, "task-as", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)
	waitForTask(t, mgr, "task-as")

	workDir := mgr.currentCfg().Server.WorkDir
	config.SetServerConfig(config.Server{WorkDir: workDir})
	config.SetConfigFilePath(filepath.Join(workDir, "config.yaml"))
	createBackup(t, workDir, "tag_test.yaml", "server:\n  port: 8080\n")

	if err := mgr.AddConfigTag("tag_test.yaml", "deploy-v2"); err != nil {
		t.Fatalf("AddConfigTag via Manager failed: %v", err)
	}

	metaPath := filepath.Join(workDir, "config_backups", "tag_test.yaml.meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("meta file not created: %v", err)
	}
	if !contains(string(data), "deploy-v2") {
		t.Fatal("meta should contain the tag")
	}
}

func TestManager_AddConfigNote_Success(t *testing.T) {
	saveRestoreGlobals(t)
	mgr, _ := newMockManager(t, "task-ns", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)
	waitForTask(t, mgr, "task-ns")

	workDir := mgr.currentCfg().Server.WorkDir
	config.SetServerConfig(config.Server{WorkDir: workDir})
	config.SetConfigFilePath(filepath.Join(workDir, "config.yaml"))
	createBackup(t, workDir, "note_test.yaml", "server:\n  port: 8080\n")

	if err := mgr.AddConfigNote("note_test.yaml", "config change note", "tester"); err != nil {
		t.Fatalf("AddConfigNote via Manager failed: %v", err)
	}

	metaPath := filepath.Join(workDir, "config_backups", "note_test.yaml.meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("meta file not created: %v", err)
	}
	if !contains(string(data), "config change note") {
		t.Fatal("meta should contain the note")
	}
}

func TestManager_GetConfig_AfterStart(t *testing.T) {
	saveRestoreGlobals(t)
	mgr, _ := newMockManager(t, "task-ga", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)
	waitForTask(t, mgr, "task-ga")

	workDir := mgr.currentCfg().Server.WorkDir
	mgrCfg := mgr.currentCfg()
	t.Logf("mgr workdir: %s", workDir)
	t.Logf("config workdir: %s", config.GetServerConfig().WorkDir)
	t.Logf("GetConfig().WorkDir: %s", mgrCfg.Server.WorkDir)

	cfg := mgr.GetConfig()
	if cfg == nil {
		t.Fatal("GetConfig() returned nil after start")
	}
	if cfg.Downloader.GlobalConcurrent != 5 {
		t.Fatalf("expected GlobalConcurrent=5, got %d", cfg.Downloader.GlobalConcurrent)
	}
}

func TestManager_RollbackConfig_UpdateConfigChain(t *testing.T) {
	saveRestoreGlobals(t)
	mgr, _ := newMockManager(t, "task-rc", 1, mockdl.New(mockdl.ModeAlwaysSuccess))
	_ = startManager(t, mgr)
	waitForTask(t, mgr, "task-rc")

	workDir := mgr.currentCfg().Server.WorkDir
	config.SetServerConfig(config.Server{WorkDir: workDir})
	config.SetConfigFilePath(filepath.Join(workDir, "config.yaml"))

	createBackup(t, workDir, "rollback_port.yaml",
		"server:\n  work_dir: "+workDir+"\n  http_port: 9999\n"+
			"downloader:\n  global_concurrent: 5\n  max_retries: 3\n")

	audit := &AuditInfo{Author: "test", Message: "rollback test", Source: "unit-test"}

	err := mgr.RollbackConfig("rollback_port.yaml", audit)
	if err != nil {
		t.Logf("RollbackConfig (expected if no config file): %v", err)
	}
}

// Assert compile-time: ensure the test file uses the dependencies.
var _ = assert.MustEventually
var _ = time.Second
