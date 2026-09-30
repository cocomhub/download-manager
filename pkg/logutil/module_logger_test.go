// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package logutil

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNewModuleLogger_WritesAppend 验证模块日志追加写（两条记录都在，非覆盖）。
func TestNewModuleLogger_WritesAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.log")
	lg := NewModuleLogger(ModuleLogConfig{Filename: path, Level: slog.LevelInfo})
	defer lg.Close()
	lg.Info("first")
	lg.Warn("second")
	// lumberjack 异步缓冲写：短暂等待落盘后读取
	waitForFile(t, path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected ≥2 appended lines, got %d: %q", len(lines), string(data))
	}
	if !strings.Contains(lines[0], "first") || !strings.Contains(lines[1], "second") {
		t.Fatalf("lines not in order: %q", string(data))
	}
}

// TestNewModuleLogger_LevelFilter 验证 debug 级别不过滤、info 过滤 debug。
func TestNewModuleLogger_LevelFilter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.log")
	lg := NewModuleLogger(ModuleLogConfig{Filename: path, Level: slog.LevelInfo})
	defer lg.Close()
	lg.Debug("should-not-appear")
	lg.Info("should-appear")
	waitForFile(t, path)
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "should-not-appear") {
		t.Fatalf("debug line leaked at info level: %q", string(data))
	}
	if !strings.Contains(string(data), "should-appear") {
		t.Fatalf("info line missing: %q", string(data))
	}
}

// waitForFile 等待 lumberjack 落盘（异步写缓冲）。
func waitForFile(t *testing.T, path string) {
	t.Helper()
	for range 20 {
		if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
			// 再等一瞬确保写完当前行
			time.Sleep(100 * time.Millisecond)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("log file never appeared: %s", path)
}
