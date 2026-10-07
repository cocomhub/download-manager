// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// progress 渲染下载进度（wget 风格：进度条 + 速度 + ETA）。
// quiet=true 时静默（仅错误输出）；非 quiet 时每次节流窗口刷一行（\r 覆盖）。
type progress struct {
	w       io.Writer
	quiet   bool
	text    string       // 输出内容（文件名）
	base    int64        // 续传起始已下字节（文件已存在部分，不上报时给 ETA 用）
	lastTm  atomic.Int64 // 上次刷新时间戳（ms）
	lastDl  atomic.Int64 // 上次刷新已下字节（用于速度差分）
	lastRt  atomic.Int64 // 上次刷新已下快照时间戳（速度窗口采样）
	beganAt time.Time    // 本次进度开始时间（平均速率）
	active  atomic.Bool
}

func newProgress(w io.Writer, quiet bool) *progress {
	return &progress{w: w, quiet: quiet}
}

// start 记录下载文件名与续传基址；report 前必须先调用。
func (p *progress) start(label string) {
	p.text = label
	p.beganAt = time.Now()
	p.active.Store(true)
}

// setBase 设置续传起始已下载字节（已有文件大小）。放在 start 之后、首次 report 之前。
func (p *progress) setBase(b int64) {
	p.base = b
}

// report 上报一次进度快照。downloaded 是本次新下载字节（不含 base）。
// 计算滑动平均速度（采样窗口 ≥500ms）与 ETA，wget 风格行内刷新。
func (p *progress) report(percent float64, downloaded, total int64) {
	if p.quiet || !p.active.Load() {
		return
	}
	now := time.Now()
	nowMs := now.UnixMilli()
	last := p.lastTm.Load()
	if nowMs-last < 500 { // 节流：≥500ms 才刷一次
		return
	}
	p.lastTm.Store(nowMs)

	eff := p.base + downloaded // 有效已下载（含续传基）

	// 速度：窗口差分（最近 ≤5s 的 delta），回退到平均速率。
	prevDl := p.lastDl.Load()
	prevTm := p.lastRt.Load()
	var speed float64
	if prevTm > 0 && prevDl > 0 {
		dt := nowMs - prevTm
		if dt >= 200 {
			speed = float64(downloaded-prevDl) * 1000 / float64(dt)
		}
	}
	if speed <= 0 {
		el := now.Sub(p.beganAt)
		if el > 0 {
			speed = float64(downloaded) / el.Seconds()
		}
	}
	p.lastDl.Store(downloaded)
	p.lastRt.Store(nowMs)

	// ETA：使用有效余量（total - base - downloaded）。
	var eta string
	if total > 0 && speed > 0 {
		remaining := total - eff
		if remaining <= 0 {
			eta = "0s"
		} else {
			eta = formatETA(time.Duration(float64(remaining)/speed) * time.Second)
		}
	} else {
		eta = "--:--"
	}

	// 进度条：%3d% [=========>----]
	bar := progressBar(int(percent))
	fmt.Fprintf(p.w, "\r[pikget] %s %s %s  %s/s  ETA %s", p.text, bar, humanize(float64(eff)), humanRate(eff, now.Sub(p.beganAt)), eta)
}

// done 输出完成摘要（换行收尾进度行）。
func (p *progress) done(ok bool, msg string) {
	p.active.Store(false)
	if p.quiet {
		return
	}
	if ok {
		fmt.Fprintf(p.w, "\r[pikget] %s: done (%s)\n", p.text, msg)
	} else {
		fmt.Fprintf(p.w, "\r[pikget] %s: failed\n", p.text)
	}
}

// progressBar 渲染 wget 风格进度条：width=20，[====>---]。
func progressBar(percent int) string {
	const w = 20
	pct := percent
	if pct > 100 {
		pct = 100
	}
	pos := (pct * w) / 100
	if pct == 100 {
		pos = w
	}
	b := make([]byte, w+2)
	b[0] = '['
	b[w+1] = ']'
	for i := 1; i <= w; i++ {
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

// humanRate 格式化平均速率（KB/s / MB/s / GB/s）。
func humanRate(bytes int64, el time.Duration) string {
	if el <= 0 {
		return "0 B/s"
	}
	return humanize(float64(bytes)/el.Seconds()) + "/s"
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
