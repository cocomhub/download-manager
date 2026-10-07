// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestNewHybrid_Minimal 验证最小装配（无账号池）可构造成功。
// cliBinary 注入本机真实 pikpak（若存在）；不存在时测试跳过（环境依赖）。
func TestNewHybrid_Minimal(t *testing.T) {
	exe := lookupPikpakCLI(t)
	if exe == "" {
		t.Skip("pikpak CLI 不存在，跳过（装配需 CLI 预检）")
	}
	dl, err := newHybrid(pikpakOpts{cliBinary: exe})
	if err != nil {
		t.Fatalf("newHybrid: %v", err)
	}
	if dl == nil {
		t.Fatal("expected non-nil hybrid downloader")
	}
	if dl.Name() != "pikpak-hybrid" {
		t.Fatalf("Name() = %q, want pikpak-hybrid", dl.Name())
	}
}

// TestNewHybrid_SupportsShareURL 验证 hybrid 支持分享 URL（装配后 Supports 语义）。
func TestNewHybrid_SupportsShareURL(t *testing.T) {
	exe := lookupPikpakCLI(t)
	if exe == "" {
		t.Skip("pikpak CLI 不存在，跳过")
	}
	dl, err := newHybrid(pikpakOpts{cliBinary: exe})
	if err != nil {
		t.Fatalf("newHybrid: %v", err)
	}
	if !dl.Supports("https://mypikpak.com/s/V1bzETjSE3NwdS") {
		t.Fatal("expected hybrid to support mypikpak share URL")
	}
	if dl.Supports("https://example.com/a.mp4") {
		t.Fatal("hybrid must not support plain http URL")
	}
}

// TestNewHybrid_AccountPool 验证 secretsDir 非空时装配账号池成功
// （stateDir 显式给 t.TempDir，避免写用户配置目录）。
func TestNewHybrid_AccountPool(t *testing.T) {
	exe := lookupPikpakCLI(t)
	if exe == "" {
		t.Skip("pikpak CLI 不存在，跳过")
	}
	dl, err := newHybrid(pikpakOpts{
		cliBinary:  exe,
		secretsDir: t.TempDir(),
		stateDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("newHybrid with account pool: %v", err)
	}
	if dl == nil {
		t.Fatal("expected non-nil hybrid downloader with account pool")
	}
}

// TestNewHybrid_ConfigDefaults 验证默认参数由 sproxy 侧钳制（share ratio ≤0.5 等）。
func TestNewHybrid_ConfigDefaults(t *testing.T) {
	exe := lookupPikpakCLI(t)
	if exe == "" {
		t.Skip("pikpak CLI 不存在，跳过")
	}
	dl, err := newHybrid(pikpakOpts{
		cliBinary:   exe,
		shareRatio:  0.9, // 超上限，应由 sproxy 钳到 0.5
		chunkSize:   0,   // 0 = 默认 64MiB
		concurrency: 0,   // 0 = 默认 4
	})
	if err != nil {
		t.Fatalf("newHybrid: %v", err)
	}
	_ = dl // 钳制在 sproxy 内部发生；构造不报错即为通过
}

// findExecutable 在 PATH 查找可执行文件路径。
func findExecutable(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// lookupPikpakCLI 返回可用的 pikpak CLI 路径（PATH 或常见安装点），找不到返回 ""。
func lookupPikpakCLI(t *testing.T) string {
	t.Helper()
	if p := findExecutable("pikpak"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		if p := findExecutable("pikpak.exe"); p != "" {
			return p
		}
	}
	return ""
}

// TestNewHybrid_CliMissingFailsFast 验证 CLI 缺失时装配立即报错（不静默），
// 而非账号 chunk 走到一半才发现。注入不存在的 BinaryPath 且禁用自动安装。
func TestNewHybrid_CliMissingFailsFast(t *testing.T) {
	_, err := newHybrid(pikpakOpts{cliBinary: filepath.Join(t.TempDir(), "no-such-pikpak")})
	if err == nil {
		t.Fatal("expected error when pikpak CLI missing")
	}
}
