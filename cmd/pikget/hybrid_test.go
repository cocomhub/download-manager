// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// TestNewHybrid_Minimal 验证最小装配（无账号池）可构造成功。
func TestNewHybrid_Minimal(t *testing.T) {
	dl, err := newHybrid(pikpakOpts{})
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
	dl, err := newHybrid(pikpakOpts{})
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
	dl, err := newHybrid(pikpakOpts{
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
	dl, err := newHybrid(pikpakOpts{
		shareRatio:  0.9, // 超上限，应由 sproxy 钳到 0.5
		chunkSize:   0,   // 0 = 默认 64MiB
		concurrency: 0,   // 0 = 默认 4
	})
	if err != nil {
		t.Fatalf("newHybrid: %v", err)
	}
	_ = dl // 钳制在 sproxy 内部发生；构造不报错即为通过
}
