// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// progress 渲染下载进度。
// quiet=true 时静默（无进度输出）。非 TTY 或 quiet 时退化为低频行日志。
type progress struct {
	w      io.Writer
	quiet  bool
	text   string // 输出内容（文件名）
	last   atomic.Int64
	active atomic.Bool
}

func newProgress(w io.Writer, quiet bool) *progress {
	return &progress{w: w, quiet: quiet}
}

// start 记录本次下载输出文件名（进度行前缀）。
func (p *progress) start(label string) {
	p.text = label
	p.active.Store(true)
}

// report 上报一次进度快照（percent 0-100）。
func (p *progress) report(percent float64, downloaded, total int64) {
	if p.quiet || !p.active.Load() {
		return
	}
	now := time.Now().UnixMilli()
	last := p.last.Load()
	if now-last < 500 { // 节流：≥500ms 才刷一次
		return
	}
	p.last.Store(now)
	pct := int(percent)
	if pct > 100 {
		pct = 100
	}
	fmt.Fprintf(p.w, "\r[pikget] %s: %3d%% (%d/%d bytes)", p.text, pct, downloaded, total)
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
