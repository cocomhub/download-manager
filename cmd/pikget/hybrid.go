// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	syncpkg "github.com/cocomhub/sproxy/pkg/sync"
	"github.com/cocomhub/sproxy/pkg/volume/ext/pikpak"
)

// pikpakOpts 是 PikPak 分享后端的运行参数。
type pikpakOpts struct {
	shareRatio  float64
	chunkSize   int64
	concurrency int
	autoDelete  bool
	secretsDir  string                 // 非空 = 装配多账号轮换（FSSecretStore 落盘目录）
	stateDir    string                 // 账号配额状态目录（默认 <UserConfigDir>/pikget/pikpak-account-state）
	cliBinary   string                 // pikpak CLI 可执行路径（空 = 自动查找/安装；测试注入 fake）
	verbose     bool                   // -v：slog 提为 Debug 级
	chunkProg   func(pikpak.ChunkInfo) // per-chunk 进度回调（pikget 逐行显示分片）
	logFile     string                 // --log-file：hybrid 日志写文件（默认丢弃）
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

	// 登录预检（fail-closed）：默认凭据 ~/.pikpak/.credentials.json（单账号）或
	// secrets 目录下 pikpak-*.json（多账号）必须存在且含 access_token。
	// 未登录立即告警，而不是账号 chunk 走到一半才发现 ErrNotLoggedIn。
	if o.secretsDir != "" {
		if err := checkAccountsLoggedIn(o.secretsDir); err != nil {
			return nil, fmt.Errorf("pikget hybrid: 账号未登录：%v", err)
		}
	} else if err := checkDefaultCredential(); err != nil {
		return nil, fmt.Errorf("pikget hybrid: 账号未登录：%v", err)
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
		Resolver:      r,
		API:           pikpak.NewAPI(pikpak.APIConfig{}, nil),
		AccountPool:   pool,
		ChunkSize:     o.chunkSize,
		ShareRatio:    o.shareRatio,
		Concurrency:   o.concurrency,
		AutoDelete:    o.autoDelete,
		Logger:        slogForPikget(o.verbose, o.logFile),
		ChunkProgress: o.chunkProg,
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

// sourceLabel 把 ChunkInfo.Source（share / acct:<name>）转为展示标签。
func sourceLabel(src string) string {
	switch {
	case src == "share":
		return "share"
	case strings.HasPrefix(src, "acct:"):
		return src // acct:name
	case src == "acct":
		return "acct"
	default:
		return src
	}
}

// defaultStateDir 返回账号配额状态默认目录。
func defaultStateDir() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", "pikget-state")
	}
	return filepath.Join(cfgDir, "pikget", "pikpak-account-state")
}

// checkDefaultCredential 校验默认单账号凭据 ~/.pikpak/.credentials.json 存在且含 access_token。
func checkDefaultCredential() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	path := filepath.Join(home, ".pikpak", ".credentials.json")
	if !fileExists(path) {
		return fmt.Errorf("默认凭据 %s 不存在——请先运行 sproxy pikpak login（OAuth 授权）", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读凭据 %s: %w", path, err)
	}
	var cred struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(b, &cred) != nil || cred.AccessToken == "" {
		return fmt.Errorf("凭据 %s 缺 access_token——请重新登录", path)
	}
	return nil
}

// checkAccountsLoggedIn 校验多账号 secrets 目录下存在至少一个 pikpak-*.json 账号凭据
// （账号池 LoadAccounts 语义：secret 名以 pikpak- 前缀）。
func checkAccountsLoggedIn(secretsDir string) error {
	if !dirExists(secretsDir) {
		return fmt.Errorf("secrets 目录 %s 不存在——请先 sproxy pikpak account add", secretsDir)
	}
	entries, err := os.ReadDir(secretsDir)
	if err != nil {
		return fmt.Errorf("读 secrets 目录 %s: %w", secretsDir, err)
	}
	found := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "pikpak-") && strings.HasSuffix(e.Name(), ".json") {
			found++
		}
	}
	if found == 0 {
		return fmt.Errorf("secrets 目录 %s 下无 pikpak-*.json 账号——请先 sproxy pikpak account add", secretsDir)
	}
	return nil
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

// dirExists 判断目录存在。
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// slogForPikget 返回 hybrid 内部日志 logger。
// **默认丢弃**（io.Discard）——日志不写终端，绝不打断进度条渲染；
// logFile 非空（--log-file）时写入文件；verbose（-v）控制文件级别 Debug（默认 Warn）。
func slogForPikget(verbose bool, logFile string) *slog.Logger {
	lvl := slog.LevelWarn
	if verbose {
		lvl = slog.LevelDebug
	}
	w := io.Writer(io.Discard)
	if logFile != "" {
		if f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			w = f
		} else {
			w = os.Stderr // 打不开文件才回落 stderr（可观测）
		}
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl}))
}
