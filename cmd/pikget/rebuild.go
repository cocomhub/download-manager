// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/cocomhub/sproxy/pkg/units/sizex"
	"github.com/cocomhub/sproxy/pkg/volume/ext/pikpak"
)

// runRebuild 实现 `pikget rebuild`——扫描/修复 PikPak 混合下载产物：
//
//	pikget rebuild [flags] <本地文件>   → 离线零尾扫描报告（不联网、不改动任何文件）
//	pikget rebuild [flags] <分享 URL>  → 在线定点重建（只补缺失/零尾可疑分片 + 权威校验 + 定稿）
//
// 退出码：0 无需重建/已修好；1 检出零尾空洞或重建失败；2 参数错误；130 中断。
func runRebuild(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("pikget rebuild", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		output     = fs.String("o", "", "输出文件路径（分享 URL 时用；默认按 URL 文件名）")
		dryRun     = fs.Bool("dry-run", false, "仅扫描报告，不修改任何文件/不下载")
		repair     = fs.Bool("repair", false, "本地文件模式下用 manifest 记录的分享 URL 联网定点重建（默认只扫描）")
		dataFile   = fs.String("data", "", "已有数据文件路径（异常命名如 *.error/*.downloading；--repair 时移入标准工作文件位置）")
		manFile    = fs.String("manifest", "", "已有进度文件路径（异常命名；--repair 时移入标准位置）")
		chunkArg   = fs.String("chunk-size", "32MiB", "分片大小（ByteSize，如 32MiB）")
		zeroRunArg = fs.String("zero-run", "64KiB", "块尾连续零判定阈值（ByteSize；≥ 该值视为未写完）")
		concur     = fs.Int("concurrency", 4, "分片并发数")
		noEscalate = fs.Bool("no-escalate", false, "补下后权威校验仍失败时不自动全量重建")
		quiet      = fs.Bool("q", false, "静默模式（仅错误）")
		verbose    = fs.Bool("v", false, "详细日志（debug 级）")
		proxyURL   = fs.String("proxy", "", "HTTP 代理（PikPak 接口/直链）")
		retry      = fs.Int("retry", 3, "接口瞬断重试次数")
		shareRatio = fs.Float64("share-ratio", 0.5, "PikPak 分享区比例（恒 ≤0.5）")
		secretsDir = fs.String("pikpak-secrets-dir", "", "PikPak 账号凭据目录（非空启用多账号）")
		noAutoRm   = fs.Bool("disable-auto-remove", false, "保留 PikPak 转存副本（默认结束自动永久删除）")
		logFile    = fs.String("log-file", "", "hybrid 日志写文件（默认丢弃）")
		configPath = fs.String("config", "", "配置文件路径（默认 ~/.config/pikget/config.yaml）")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "用法: pikget rebuild [flags] <本地文件|分享URL>\n\nFlags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(stderr, "\n说明: 块内尾部存在连续 0x00（≥ --zero-run）即视为未写完的异常块需重建；\n")
		fmt.Fprintf(stderr, "      本地文件只扫描报告；分享 URL 定点重建（只补异常分片，权威哈希校验后定稿）。\n")
		fmt.Fprintf(stderr, "\n退出码: 0无需重建/已修好 1检出空洞或重建失败 2参数错误 130中断\n")
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		fmt.Fprintf(stderr, "pikget rebuild: %v\n", err)
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(stderr, "pikget rebuild: 需要恰好一个 <本地文件|分享URL> 参数\n")
		fs.Usage()
		return exitUsage
	}
	chunkSize, err := sizex.ParseSize(*chunkArg)
	if err != nil || chunkSize <= 0 {
		fmt.Fprintf(stderr, "pikget rebuild: --chunk-size %q 非法: %v\n", *chunkArg, err)
		return exitUsage
	}
	zeroRun, err := sizex.ParseSize(*zeroRunArg)
	if err != nil || zeroRun <= 0 {
		fmt.Fprintf(stderr, "pikget rebuild: --zero-run %q 非法: %v\n", *zeroRunArg, err)
		return exitUsage
	}
	target := fs.Arg(0)
	if (*dataFile != "" || *manFile != "") && !*repair {
		fmt.Fprintf(stderr, "pikget rebuild: --data/--manifest 会移动文件，必须与 --repair 一起使用（默认只扫描）\n")
		return exitUsage
	}

	// 本地文件：默认离线扫描（不联网/不碰凭据）；--repair 时用 manifest 记录的分享 URL 联网定点重建。
	localDest := ""
	if dispatch(target) != kindPikpak {
		if !fileExists(target) {
			fmt.Fprintf(stderr, "pikget rebuild: %s 既不是 PikPak 分享 URL，也不是已存在的本地文件\n", target)
			return exitUsage
		}
		if !*repair {
			return runOfflineRebuild(target, chunkSize, zeroRun, *quiet, stdout, stderr)
		}
		localDest = pikpak.NormalizeWorkDest(target)
		if *output != "" { // 显式 -o 优先（否则用工作/成品文件名推导最终名）
			resolved, rerr := resolveDest(target, *output)
			if rerr != nil {
				fmt.Fprintf(stderr, "pikget: %v\n", rerr)
				return exitFail
			}
			localDest = resolved
		}
		manifestURL, ok := pikpak.SourceURLFromManifest(target)
		if !ok {
			fmt.Fprintf(stderr, "pikget rebuild: %s 的 manifest 未记录分享 URL（或已丢失）——请显式指定：pikget rebuild -o %s <share-url>\n", target, localDest)
			return exitFail
		}
		target = manifestURL // --repair：用 manifest 里的分享 URL 联网重建
	}

	// 分享 URL（显式或来自 manifest）→ 在线定点重建。
	cfg, cerr := loadConfig(*configPath)
	if cerr != nil {
		fmt.Fprintf(stderr, "pikget: 加载配置: %v\n", cerr)
		return exitFail
	}
	dest := localDest
	if dest == "" {
		var derr error
		dest, derr = resolveDest(target, *output)
		if derr != nil {
			fmt.Fprintf(stderr, "pikget: %v\n", derr)
			return exitFail
		}
	}
	opts := pikpakOpts{
		shareRatio:  *shareRatio,
		chunkSize:   chunkSize,
		zeroRun:     zeroRun,
		concurrency: *concur,
		autoDelete:  !*noAutoRm && !cfg.Downloader.Pikpak.DisableAutoRemove,
		secretsDir:  *secretsDir,
		verbose:     *verbose,
		logFile:     *logFile,
		proxyURL:    or(*proxyURL, cfg.Downloader.HTTP.Proxy),
		retry:       *retry,
		noEscalate:  *noEscalate,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// 异常命名的数据/进度文件先归位（用户常把失败产物改名成 *.error 手工标记）。
	if err := stageRebuildInputs(dest, *dataFile, *manFile); err != nil {
		fmt.Fprintf(stderr, "pikget rebuild: %v\n", err)
		return exitFail
	}
	rep, rerr := rebuildHybrid(ctx, target, dest, opts, *dryRun, nil)
	v := rebuildViewFromReport(dest, rep, *dryRun)
	if !*quiet {
		printRebuildReport(stdout, v)
	}
	if rerr != nil {
		fmt.Fprintf(stderr, "pikget: %v\n", rerr)
		return exitFail
	}
	if *dryRun {
		if len(v.Rebuild) > 0 || len(v.Suspects) > 0 {
			return exitFail // 检出待重建分片（dry-run 不改动）
		}
		return exitOK
	}
	if !v.Finalized {
		fmt.Fprintf(stderr, "pikget rebuild: 未定稿（文件仍需修复）\n")
		return exitFail
	}
	return exitOK
}

// stageRebuildInputs 把异常命名的数据/进度文件归位到标准工作位置（领域能力实现）。
func stageRebuildInputs(dest, dataPath, manifestPath string) error {
	return pikpak.StageWorkFiles(dest, dataPath, manifestPath)
}

// runOfflineRebuild 离线扫描本地文件（不联网）：扫描/可信判定由 sproxy 领域能力完成
// （pikpak.OfflineScan：manifest ∩ 零尾扫描），本函数只负责 CLI 输出与退出码。
func runOfflineRebuild(path string, chunkSize, zeroRun int64, quiet bool, stdout, stderr io.Writer) int {
	rep, err := pikpak.OfflineScan(path, chunkSize, zeroRun)
	if err != nil {
		fmt.Fprintf(stderr, "pikget rebuild: %v\n", err)
		return exitFail
	}
	view := rebuildView{
		Target: path, DataPath: rep.DataPath, Total: rep.Total, ChunkSize: rep.ChunkSize,
		ChunksTotal: rep.ChunksTotal, ChunksTrust: rep.ChunksTrust,
		Suspects: rep.Suspects, Rebuild: rep.Rebuild,
		DryRun: true, Offline: rep.Manifest == "",
		Manifest: rep.Manifest, ManifestDone: rep.ManifestDone,
	}
	if !quiet {
		printRebuildReport(stdout, view)
	}
	if len(rep.Suspects) > 0 {
		return exitFail // 检出需要重建的分片（离线扫描不改动文件）
	}
	return exitOK
}

// rebuildView 是重建/扫描报告的统一展示视图（离线扫描与在线重建共用）。
type rebuildView struct {
	Target       string
	DataPath     string
	Total        int64
	ChunkSize    int64
	ChunksTotal  int
	ChunksTrust  int
	Rebuild      []pikpak.ChunkSpan
	Suspects     []pikpak.ZeroTailSuspect
	HashVerified bool
	FullRebuild  bool
	Finalized    bool
	DryRun       bool
	Offline      bool
	Manifest     string
	ManifestDone int
}

// rebuildViewFromReport 把 sproxy 报告转成展示视图。
func rebuildViewFromReport(dest string, rep *pikpak.RebuildReport, dryRun bool) rebuildView {
	v := rebuildView{Target: dest, Total: 0, ChunkSize: 0, DryRun: dryRun}
	if rep == nil {
		return v
	}
	v.DataPath, v.Total, v.ChunkSize = rep.DataPath, rep.Total, rep.ChunkSize
	v.ChunksTotal, v.ChunksTrust = rep.ChunksTotal, rep.ChunksTrust
	v.Rebuild, v.Suspects = rep.RebuildSpans, rep.ZeroSuspect
	v.HashVerified, v.FullRebuild, v.Finalized = rep.HashVerified, rep.FullRebuild, rep.Finalized
	v.Manifest = pikpak.ManifestPath(dest)
	return v
}

// printRebuildReport 输出扫描/重建报告（人可读，中文；不修改文件）。
func printRebuildReport(w io.Writer, v rebuildView) {
	mode := "重建"
	if v.DryRun {
		mode = "扫描（--dry-run，未改动）"
	}
	fmt.Fprintf(w, "pikget rebuild: %s   [%s]\n", v.Target, mode)
	fmt.Fprintf(w, "  数据文件: %s\n", or(v.DataPath, v.Target))
	fmt.Fprintf(w, "  大小: %s  分块: %s × %d 片\n", humanize(float64(v.Total)), humanize(float64(v.ChunkSize)), v.ChunksTotal)
	if v.Manifest != "" || v.ManifestDone > 0 {
		fmt.Fprintf(w, "  进度文件: %s（记录 %d 片）\n", v.Manifest, v.ManifestDone)
	} else {
		fmt.Fprintf(w, "  进度文件: 无 manifest（total 取文件大小）\n")
	}
	if v.ChunksTrust > 0 || v.ChunksTotal > 0 {
		fmt.Fprintf(w, "  可信分片: %d / %d\n", v.ChunksTrust, v.ChunksTotal)
	}
	if v.HashVerified {
		fmt.Fprintf(w, "  权威校验: 现有数据已与源一致（无需下载）\n")
	}
	fmt.Fprintf(w, "  零尾可疑块: %d（块尾连续 0x00 ≥ 阈值 = 未写完）\n", len(v.Suspects))
	for _, s := range v.Suspects {
		fmt.Fprintf(w, "    %s 零尾 %s\n", chunkSpanLabel(pikpak.ChunkSpan{Offset: s.Offset, Length: s.Length}, v.ChunkSize), humanize(float64(s.Zeros)))
	}
	if len(v.Rebuild) > 0 {
		fmt.Fprintf(w, "  待重建分片: %d\n", len(v.Rebuild))
		for _, span := range v.Rebuild {
			fmt.Fprintf(w, "    %s\n", chunkSpanLabel(span, v.ChunkSize))
		}
	} else {
		fmt.Fprintf(w, "  待重建分片: 0 —— 无需重建\n")
	}
	switch {
	case v.FullRebuild:
		fmt.Fprintf(w, "  结果: 权威校验首次未通过 → 已全量重建并定稿 → %s\n", v.Target)
	case v.Finalized:
		fmt.Fprintf(w, "  结果: 已定稿 → %s\n", v.Target)
	case v.DryRun:
		fmt.Fprintf(w, "  结果: 仅扫描（未改动文件）\n")
	}
	if !v.Finalized {
		if v.Offline {
			fmt.Fprintf(w, "  恢复建议: pikget rebuild -o %s <share-url>  （联网定点重建；重跑下载亦可续传）\n", v.Target)
		} else {
			fmt.Fprintf(w, "  恢复建议: 重跑 pikget <share-url> -o %s（自动续传）\n", v.Target)
		}
	}
}

// chunkSpanLabel 渲染「#编号 @offset +length」标签（编号与 sproxy 分片序号对齐）。
func chunkSpanLabel(span pikpak.ChunkSpan, chunkSize int64) string {
	idx := int64(-1)
	if chunkSize > 0 {
		idx = span.Offset / chunkSize
	}
	if idx >= 0 {
		return fmt.Sprintf("#%d @%d +%d", idx, span.Offset, span.Length)
	}
	return fmt.Sprintf("@%d +%d", span.Offset, span.Length)
}

// printHybridFailureHint 输出失败后的可执行恢复指引（分片号/续传/rebuild）——
// 用户实测痛点：只给一行网络错误，不知道「下到哪了/要不要重下/怎么修」。
func printHybridFailureHint(w io.Writer, err error, dest string, chunkSize int64) {
	fmt.Fprintf(w, "pikget: 下载未完成 —— 已下载分片保留（.hybrid 进度 + .hybrid.downloading 数据），重跑同一条命令即可续传\n")
	if ce, ok := errors.AsType[*pikpak.ChunkError](err); ok {
		fmt.Fprintf(w, "  失败分片: %s\n", chunkSpanLabel(pikpak.ChunkSpan{Offset: ce.Offset, Length: ce.Length}, chunkSize))
		if isTransientFailure(ce.Err) {
			fmt.Fprintln(w, "  说明: 取链/传输层瞬断（TLS 握手超时、连接重置等）——重跑会重新取链并续传，无需重下已有分片")
		}
	}
	fmt.Fprintf(w, "  定点修复: pikget rebuild -o %s <share-url>\n", dest)
	fmt.Fprintf(w, "  仅扫描  : pikget rebuild %s\n", dest)
}

// isTransientFailure 判断错误是否属传输层瞬断（net.Error：握手超时/连接重置等）——
// 用于向用户解释「重跑即可续传，不是数据坏了」。
func isTransientFailure(err error) bool {
	var ne net.Error
	return errors.As(err, &ne)
}
