// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// workerStat 是单个执行者（chunk）的进度状态。
type workerStat struct {
	id     string // 执行者代号（chunk 标识）
	name   string // 展示名（文件名/序号）
	total  int64  // 总大小
	done   int64  // 已下字节（原子更新，加锁读快照）
	base   int64  // 续传起始字节（manifest 累计）
	finish bool   // 是否完成（移除速率/ETA）
	// 速率窗口
	speed  float64 // 最近速度（B/s）
	lastTm int64   // 上次刷新时间戳（ms）
}

// multiProgress 是并发安全的多执行者进度条（docker pull 分层风格）。
// - 每执行者一行，固定列宽（%-widths 填充，长度变化不残留）
// - 每行显示：进度条 + 百分比 + 字节 + 速率 + ETA；完成移除速率/ETA
// - 最后一行汇总所有执行者的总进度/速率/ETA
// - Update(id, n) 传增量字节，内部自动对齐对应 chunk
//
// 终端能力分级：
//   - ansi（Windows Terminal / Linux / macOS 终端）：多行原地刷新（\x1b[A 上移）
//   - 非 ansi（经典 conhost / PowerShell 5.1）：退化为单行汇总原地刷新（\r+\x1b[K），
//     避免滚屏——worker 明细由 -v 时 sproxy chunk 日志呈现
type multiProgress struct {
	w       io.Writer
	mu      sync.Mutex // 保护 workers 快照与渲染
	workers map[string]*workerStat
	enabled bool     // 显示开关（TTY 或非 quiet 才画）
	order   []string // id 稳定顺序（创建序）
	widths  int      // 名称列宽（固定，避免长度抖动残留）
	refresh time.Time
}

// newMultiProgress 创建多执行者进度条。enabled=false 时 Update/Done 为 no-op。
// ansi 能力由终端探测决定：Windows Terminal 或非 Windows → 多行；经典 conhost → 单行汇总。
func newMultiProgress(w io.Writer, enabled bool) *multiProgress {
	return &multiProgress{
		w:       w,
		enabled: enabled,
		workers: make(map[string]*workerStat),
		widths:  20,
	}
}

// isTTY 判断 writer 是否为字符设备（终端）。非终端（管道/重定向/测试 buffer）→ false，
// 调用方据此禁用进度动画（避免 ANSI 控制码污染管道输出与捕获断言）。
// 用 golang.org/x/term.IsTerminal（跨平台：Windows conhost 也正确识别）替代手写 Stat。
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// addWorker 注册一个执行者（chunk）。base 为续传起始字节；total 总大小；name 展示名。
func (p *multiProgress) addWorker(id string, total, base int64, name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.workers[id]; ok {
		return // 已存在（幂等）
	}
	p.workers[id] = &workerStat{
		id: id, name: name, total: total, base: base,
		lastTm: time.Now().UnixMilli(),
	}
	p.order = append(p.order, id)
	if len(name) > p.widths {
		p.widths = len(name)
	}
}

// update 更新某执行者的新写入长度（增量字节）。
func (p *multiProgress) update(id string, n int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	w, ok := p.workers[id]
	if !ok || w.finish {
		return
	}
	w.done += n
	p.refreshLocked()
}

// set 设置某执行者的绝对进度（downloaded/total）。total<=0 时保留现有。
// 首次 total>0 时填充该 worker 的 total（调用方在 addWorker 时未知总大小）。
func (p *multiProgress) set(id string, downloaded, total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	w, ok := p.workers[id]
	if !ok {
		return
	}
	if total > 0 {
		w.total = total
	}
	if downloaded > w.done {
		w.done = downloaded
	}
	p.refreshLocked()
}

// refreshLocked 检查节流并渲染（调用方持锁）。
func (p *multiProgress) refreshLocked() {
	if !p.enabled {
		return
	}
	now := time.Now()
	if now.Sub(p.refresh) < 300*time.Millisecond {
		return
	}
	p.refresh = now
	p.render(now)
}

// render 单行汇总原地刷新（wget 同款：\r + 空格填充，零 ANSI）。
func (p *multiProgress) render(now time.Time) {
	// wget/docker 同款单行刷新: CR 回行首 + 空格填充覆盖. 零 ANSI.
	fmt.Fprintf(p.w, "\r%s", pad(p.summary(now), 100))
}

// speedOf 计算某执行者最近速度（窗口差分 ≥300ms，回退平均）。
// summary 渲染汇总行：总进度条 + % + 字节 + 速率 + ETA。
func (p *multiProgress) summary(now time.Time) string {
	var totalDone, totalBase, totalSize int64
	doneCnt := 0
	for _, id := range p.order {
		w := p.workers[id]
		totalBase += w.base
		totalDone += w.done
		totalSize += w.total
		if w.finish {
			doneCnt++
		}
	}
	eff := totalBase + totalDone
	pct := float64(eff) / float64(totalSize) * 100
	if pct > 100 {
		pct = 100
	}
	// 汇总速率 = 总字节差分（简化：按最近窗口总增）
	speed := p.totalSpeed()
	eta := "--:--"
	if speed > 0 && totalSize-eff > 0 {
		eta = formatETA(time.Duration(float64(totalSize-eff)/speed) * time.Second)
	}
	status := "downloading"
	if doneCnt == len(p.order) {
		status = "done"
	}
	return fmt.Sprintf(" %s %s %5.1f%% %s %s %12s %s",
		pad("total", p.widths), progressBar(int(pct), 20), pct,
		humanize(float64(eff)), fmt.Sprintf("/ %s", humanize(float64(totalSize))),
		humanRate(speed), eta+" "+status)
}

// totalSpeed 汇总速度：所有未完成 worker 最近速度之和（避免差分窗口混乱，用缓存 speed）。
func (p *multiProgress) totalSpeed() float64 {
	var sum float64
	for _, id := range p.order {
		w := p.workers[id]
		if !w.finish {
			sum += w.speed
		}
	}
	return sum
}

// finish 收尾：TTY 单行摘要（wget 同款：CR + 空格填充，零 ANSI）；非 TTY 静默。
func (p *multiProgress) finish(ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.enabled {
		return
	}
	// wget 同款：\r + 空格填充覆盖 + 摘要（无 ANSI，任何终端正确）
	if isTTY(p.w) && ok {
		s := p.summary(time.Now())
		fmt.Fprintf(p.w, "\r%s\n", pad(strings.TrimSpace(s), 100))
	}
}

// pad 按固定宽度填充（右侧补空格，避免长度变化残留）。
func pad(s string, w int) string {
	if len(s) >= w {
		return s[:w]
	}
	return s + strings.Repeat(" ", w-len(s))
}

// progressBar 渲染进度条（width 格）。
func progressBar(percent, width int) string {
	pct := min(percent, 100)
	b := make([]byte, width+2)
	b[0] = '['
	b[width+1] = ']'
	if pct >= 100 {
		for i := 1; i <= width; i++ {
			b[i] = '='
		}
		return string(b)
	}
	pos := (pct * width) / 100
	for i := 1; i <= width; i++ {
		switch {
		case i < pos:
			b[i] = '='
		case i == pos:
			b[i] = '>'
		default:
			b[i] = '-'
		}
	}
	return string(b)
}

// formatETA 把 Duration 转 mm:ss（≥1h 显示 h:mm:ss）。
func formatETA(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// humanRate 格式化速率（B/s / KB/s / MB/s）。
func humanRate(speed float64) string {
	if speed <= 0 {
		return "0 B/s"
	}
	return humanize(speed) + "/s"
}

// humanize 人类可读字节数（B/KB/MB/GB），保留 1 位小数。
func humanize(v float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	u := 0
	for v >= 1024 && u < len(units)-1 {
		v /= 1024
		u++
	}
	if u == 0 {
		return fmt.Sprintf("%d %s", int64(v), units[u])
	}
	return fmt.Sprintf("%.1f %s", v, units[u])
}
