// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

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
}

// newHybrid 装配 sproxy HybridDownloader。
// 最小装配 = Resolver + API（单账号/CLI 会话现状）；secretsDir 非空时加装配
// AccountPool（多账号配额轮换）。
func newHybrid(o pikpakOpts) (*pikpak.HybridDownloader, error) {
	r := pikpak.NewShareResolver(pikpak.ShareResolverConfig{})

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

// slogForPikget 返回默认 slog logger（hybrid 内部日志到 stderr，-v 时降噪可切 debug）。
func slogForPikget() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}
