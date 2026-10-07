// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// pikget 是一个类 wget 的下载器：
//   - 普通 http(s) 直链 → download-manager pkg/download（Range 续传 / ETag / MD5）
//   - PikPak 分享链接（mypikpak/keepshare）→ sproxy pikpak 混合下载（分享直链前段 + 账号流量后段）
//   - 磁力 / 其它 scheme → 报「暂不支持」
//
// 退出码：0 成功 / 1 下载失败或不支持 / 2 参数错误 / 130 中断。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
	exitInt   = 130
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// cliFlags 是解析后的命令参数（flag 与 config 合并）。
type cliFlags struct {
	output     string
	quiet      bool
	noProgress bool // --no-progress：禁用多行进度条（默认开启）
	verbose    bool
	userAgent  string
	proxyURL   string
	headers    []string
	timeoutSec int
	retry      int
	// pikpak
	shareRatio  float64
	chunkSize   int64
	concurrency int
	autoDelete  bool
	secretsDir  string
}

// run 是命令入口（可测试：返回退出码，不直接 os.Exit）。
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("pikget", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		output     = fs.String("o", "", "输出文件路径或目录（默认：URL 文件名到当前目录）")
		quiet      = fs.Bool("q", false, "静默模式（仅错误输出）")
		noProgress = fs.Bool("no-progress", false, "禁用多行进度条（默认开启）")
		verbose    = fs.Bool("v", false, "详细日志（debug 级）")
		userAgent  = fs.String("user-agent", "", "直链 User-Agent")
		proxyURL   = fs.String("proxy", "", "直链 HTTP 代理")
		headerArg  = fs.String("header", "", "自定义请求头 'K: V'（可重复/逗号分隔）")
		timeout    = fs.Int("timeout", 300, "HTTP 超时(秒)")
		retry      = fs.Int("retry", 3, "重试次数")
		configPath = fs.String("config", "", "配置文件路径（默认 ~/.config/pikget/config.yaml）")
		shareRatio = fs.Float64("share-ratio", 0.5, "PikPak 分享区比例（恒 ≤0.5）")
		chunkSize  = fs.Int64("chunk-size", 64<<20, "PikPak 分片大小(字节)")
		concur     = fs.Int("concurrency", 4, "并发数")
		autoDelete = fs.Bool("disable-auto-remove", false, "保留 PikPak 转存副本（默认结束自动永久删除）")
		secretsDir = fs.String("pikpak-secrets-dir", "", "PikPak 账号凭据目录（非空启用多账号）")
		showVer    = fs.Bool("version", false, "显示版本")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "用法: pikget [flags] <URL>\n\nFlags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(stderr, "\n退出码: 0成功 1失败/不支持 2参数错误 130中断\n")
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		fmt.Fprintf(stderr, "pikget: %v\n", err)
		return exitUsage
	}
	if *showVer {
		fmt.Fprintf(stdout, "pikget %s\n", version)
		return exitOK
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(stderr, "pikget: 需要恰好一个 <url> 参数\n")
		fs.Usage()
		return exitUsage
	}

	// 配置（flag 默认值与 config.yaml 合并；显式 flag 优先）
	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "pikget: 加载配置: %v\n", err)
		return exitFail
	}
	fl := cliFlags{
		output:      *output,
		quiet:       *quiet,
		noProgress:  *noProgress,
		verbose:     *verbose,
		userAgent:   or(*userAgent, cfg.Downloader.HTTP.UserAgent),
		proxyURL:    or(*proxyURL, cfg.Downloader.HTTP.Proxy),
		timeoutSec:  *timeout,
		retry:       *retry,
		shareRatio:  *shareRatio,
		chunkSize:   *chunkSize,
		concurrency: *concur,
		// 配置纪律：flag --disable-auto-remove 默认 false；config.yaml 同名字段同步。
		// 默认（两者皆未设）→ autoDelete=true（结束自动删转存，安全默认）。
		autoDelete: !*autoDelete && !cfg.Downloader.Pikpak.DisableAutoRemove,
		secretsDir: *secretsDir,
	}
	if len(cfg.Downloader.HTTP.Headers) > 0 {
		fl.headers = append(fl.headers, cfg.Downloader.HTTP.Headers...)
	}
	if *headerArg != "" {
		fl.headers = append(fl.headers, splitHeaders(*headerArg)...)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	url := fs.Arg(0)
	switch dispatch(url) {
	case kindDirect:
		return runDirect(ctx, url, fl, stdout, stderr)
	case kindPikpak:
		return runHybrid(ctx, url, fl, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "pikget: 暂不支持该 URL: %s\n", url)
		return exitFail
	}
}

// or 返回第一个非空字符串。
func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// splitHeaders 解析 'K: V; K2: V2' 形式的请求头。
func splitHeaders(s string) []string {
	var out []string
	rest := s
	for rest != "" {
		rest = trimLeft(rest)
		if rest == "" {
			break
		}
		idx := indexAny(rest, ";")
		var seg string
		if idx < 0 {
			seg, rest = rest, ""
		} else {
			seg, rest = rest[:idx], rest[idx+1:]
		}
		if seg != "" {
			out = append(out, trimSpace(seg))
		}
	}
	return out
}

// runDirect 执行直链下载。
// 直链后端 pkg/download 是单流（无分片），显示 1 行总进度（multiProgress 架构支持 N worker，
// 待后端暴露 per-chunk 回调时自动多行）。
func runDirect(ctx context.Context, url string, fl cliFlags, stdout, stderr io.Writer) int {
	dest, err := resolveDest(url, fl.output)
	if err != nil {
		fmt.Fprintf(stderr, "pikget: %v\n", err)
		return exitFail
	}
	if !fl.quiet {
		fmt.Fprintf(stdout, "pikget: 直链下载 %s -> %s\n", url, dest)
	}
	pr := newMultiProgress(stdout, !fl.quiet && !fl.noProgress && isTTY(stdout))
	pr.addWorker("main", 0, 0, filepath.Base(dest)) // total 未知，首次回调填充
	// 直链进度：pkg/download 回调的 dl 含续传 base（ProgressReader downloaded 初值 = offset）
	opts := httpOpts{
		userAgent: fl.userAgent,
		proxyURL:  fl.proxyURL,
		headers:   parseHeaders(fl.headers),
		maxRetry:  fl.retry,
	}
	err = downloadDirect(ctx, url, dest, opts, func(p float64, dl, total int64) {
		pr.set("main", dl, total)
	})
	if ctxErr(ctx) {
		fmt.Fprintf(stderr, "pikget: 中断\n")
		return exitInt
	}
	if err != nil {
		pr.finish(false)
		fmt.Fprintf(stderr, "pikget: %v\n", err)
		return exitFail
	}
	pr.set("main", fileSize(dest), fileSize(dest))
	pr.finish(true)
	// 非 TTY：打印 wget 风格最终摘要（含 done 字样，测试断言锁定）
	if !isTTY(stdout) && !fl.quiet {
		fmt.Fprintf(stdout, "pikget: done %s (%d bytes)\n", filepath.Base(dest), fileSize(dest))
	}
	return exitOK
}

// runHybrid 执行 PikPak 分享混合下载。
// hybrid 回调是聚合进度（sproxy 内部多分片并行，onProgress 只暴露总 downloaded/total），
// 显示 1 行汇总；-v 时 sproxy chunk 级日志（hybrid chunk done）到 stderr 可见分片明细。
func runHybrid(ctx context.Context, url string, fl cliFlags, stdout, stderr io.Writer) int {
	dest, err := resolveDest(url, fl.output)
	if err != nil {
		fmt.Fprintf(stderr, "pikget: %v\n", err)
		return exitFail
	}
	if !fl.quiet {
		fmt.Fprintf(stdout, "pikget: PikPak 混合下载 %s -> %s\n", url, dest)
	}
	pr := newMultiProgress(stdout, !fl.quiet && !fl.noProgress && isTTY(stdout))
	// 续传基准：manifest 累计已完成字节（预分配文件本身是 total，不能用文件大小）
	pr.addWorker("total", 0, hybridResumeBase(dest), filepath.Base(dest))
	opts := pikpakOpts{
		shareRatio:  fl.shareRatio,
		chunkSize:   fl.chunkSize,
		concurrency: fl.concurrency,
		autoDelete:  fl.autoDelete,
		secretsDir:  fl.secretsDir,
		verbose:     fl.verbose,
	}
	err = downloadHybrid(ctx, url, dest, opts, func(downloaded, total int64) {
		pr.set("total", downloaded, total)
	})
	if ctxErr(ctx) {
		fmt.Fprintf(stderr, "pikget: 中断\n")
		return exitInt
	}
	if err != nil {
		pr.finish(false)
		fmt.Fprintf(stderr, "pikget: %v\n", err)
		return exitFail
	}
	pr.set("total", fileSize(dest), fileSize(dest))
	pr.finish(true)
	return exitOK
}

// resolveDest 把 -o 参数解析为最终文件路径：
// -o 是目录 → <dir>/<url 文件名>；-o 是文件 → 原样；空 → ./<url 文件名>。
func resolveDest(url, output string) (string, error) {
	name := filepath.Base(url)
	if name == "" || name == "/" || name == "." {
		name = "download.bin"
	}
	if output == "" {
		return name, nil
	}
	if info, err := os.Stat(output); err == nil && info.IsDir() {
		return filepath.Join(output, name), nil
	}
	return output, nil
}

// parseHeaders 把 'K: V' 字符串列表转为 map。
func parseHeaders(pairs []string) map[string]string {
	out := make(map[string]string)
	for _, p := range pairs {
		for i := 0; i < len(p); i++ {
			if p[i] == ':' {
				out[trimSpace(p[:i])] = trimSpace(p[i+1:])
				break
			}
		}
	}
	return out
}

// hybridResumeBase 读取 dest+latestWatch".hybrid" manifest，累计已完成 chunk 字节作为续传基准。
// 预分配文件本身就是 total（os.Truncate 全量），不能右文件大小当基准——只能用 manifest 已下 chunk 之和。
func hybridResumeBase(dest string) int64 {
	b, err := os.ReadFile(dest + ".hybrid")
	if err != nil {
		return 0
	}
	var m struct {
		Chunks map[string]int64 `json:"chunks"` // offset → length
	}
	if json.Unmarshal(b, &m) != nil {
		return 0
	}
	var base int64
	for _, ln := range m.Chunks {
		base += ln
	}
	return base
}

// fileSize 返回文件大小（不存在返回 0）。
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// ctxErr 判断是否因 ctx 取消而退出（Ctrl-C / SIGTERM）。
func ctxErr(ctx context.Context) bool {
	return ctx.Err() != nil
}

// 小型字符串工具（避免引入 strconv 等）：trimSpace/trimLeft/indexAny。
func trimSpace(s string) string { return trimRight(trimLeft(s)) }

func trimLeft(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[i:]
}

func trimRight(s string) string {
	i := len(s)
	for i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
		i--
	}
	return s[:i]
}

func indexAny(s string, chars string) int {
	for i := 0; i < len(s); i++ {
		for j := 0; j < len(chars); j++ {
			if s[i] == chars[j] {
				return i
			}
		}
	}
	return -1
}
