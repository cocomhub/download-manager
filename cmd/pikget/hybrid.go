// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	syncpkg "github.com/cocomhub/sproxy/pkg/sync"
	"github.com/cocomhub/sproxy/pkg/volume/ext/pikpak"
)

// pikpakOpts 是 PikPak 分享后端的运行参数。
type pikpakOpts struct {
	shareRatio  float64
	chunkSize   int64
	concurrency int
	autoDelete  bool
	secretsDir  string // 非空 = 装配多账号轮换（FSSecretStore 落盘目录）
	stateDir    string // 账号配额状态目录（默认 <UserConfigDir>/pikget/pikpak-account-state）
	cliBinary   string // pikpak CLI 可执行路径（空 = 自动查找/安装；测试注入 fake）
}

// newHybrid 装配 sproxy HybridDownloader。
// 最小装配 = Resolver + API（单账号/CLI 会话现状）；secretsDir 非空时加装配
// AccountPool（多账号配额轮换）。
//
// 立即告警（用户要求）：装配前先定位 pikpak CLI——账号区依赖它（OAuth 会话/凭据刷新），
// CLI 缺失时在下载开始前就报错，而不是账号 chunk 走到一半才发现。
func newHybrid(o pikpakOpts) (*pikpak.HybridDownloader, error) {
	r := pikpak.NewShareResolver(pikpak.ShareResolverConfig{})

	// CLI 预检：账号区依赖官方 pikpak CLI（OAuth 会话/凭据刷新）。
	// 缺失时在下载开始前就报错（fail-closed），而不是账号 chunk 走到一半才发现。
	// 显式路径未设置 → 查 PATH；查不到即报错（不自动安装——CLI 场景应显式安装）。
	cliPath := o.cliBinary
	if cliPath == "" {
		cliPath = lookupPikpakBinary()
	}
	if cliPath == "" || !fileExists(cliPath) {
		return nil, fmt.Errorf("pikget hybrid: pikpak CLI 不可用（账号区依赖）——请先安装 pikpak CLI 到 PATH（sproxy pikpak cli install）或 --pikpak-cli <path> 指定")
	}
	var pool *pikpak.AccountPool
	if o.secretsDir != "" {
		if err := os.MkdirAll(o.secretsDir, 0o700); err != nil {
			return nil, fmt.Errorf("pikget hybrid: create secrets dir: %w", err)
		}
		store := pikpak.NewFSSecretStore(syncpkg.NewLocalFS(o.secretsDir, nil))
		p, err := pikpak.NewAccountPool(pikpak.AccountPoolConfig{
			Secrets: store,
			StateDir: func() string {
				if o.stateDir != "" {
					return o.stateDir
				}
				return defaultStateDir()
			}(),
		})
		if err != nil {
			return nil, fmt.Errorf("pikget hybrid: account pool: %w", err)
		}
		pool = p
	}

	return pikpak.NewHybridDownloader(pikpak.HybridConfig{
		Resolver:    r,
		API:         pikpak.NewAPI(pikpak.APIConfig{}, nil),
		AccountPool: pool,
		ChunkSize:   o.chunkSize,
		ShareRatio:  o.shareRatio,
		Concurrency: o.concurrency,
		AutoDelete:  o.autoDelete,
		Logger:      slogForPikget(),
	})
}

// downloadHybrid 调用 sproxy HybridDownloader 下载分享 URL 到 dest 文件。
// onProg 收到字节语义回调（downloaded, total），sproxy ProgressFunc 同款。
func downloadHybrid(ctx context.Context, shareURL, dest string, o pikpakOpts, onProg func(downloaded, total int64)) error {
	dl, err := newHybrid(o)
	if err != nil {
		return err
	}
	if _, err := dl.Download(ctx, shareURL, dest, onProg); err != nil {
		return fmt.Errorf("pikget hybrid: %w", err)
	}
	return nil
}

// defaultStateDir 返回账号配额状态默认目录。
func defaultStateDir() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", "pikget-state")
	}
	return filepath.Join(cfgDir, "pikget", "pikpak-account-state")
}

// lookupPikpakBinary 在 PATH 查 pikpak 可执行文件（无测试注入时）。
func lookupPikpakBinary() string {
	name := "pikpak"
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("pikpak.exe"); err == nil {
			return p
		}
	}
	return ""
}

// fileExists 判断文件存在。
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// slogForPikget 返回默认 slog logger（hybrid 内部日志到 stderr，-v 时降噪可切 debug）。
func slogForPikget() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}
