// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/pkg/logutil"
)

// ProgressReport 是单次进度快照的值对象，包含计算格式化日志行所需的全部字段。
type ProgressReport struct {
	Timestamp  time.Time     // 该次回调的时间戳
	Progress   float64       // 0-100 的完成百分比
	Downloaded int64         // 已下载字节数
	Total      int64         // 总字节数（保证 >0）
	Speed      float64       // 自上次回调以来的平均速度（bytes/s）
	Elapsed    time.Duration // 自首次回调以来的已用时间
	ETA        time.Duration // 预计剩余时间（当 Speed=0 时为 0）
}

// ProgressLogOption 配置 NewProgressLogCallback 的行为。
type ProgressLogOption func(*progressLogConfig)

type progressLogConfig struct {
	w           io.Writer
	minStep     float64
	maxInterval time.Duration
	formatter   func(io.Writer, ProgressReport)
}

func defaultConfig() *progressLogConfig {
	return &progressLogConfig{
		w:           io.Discard,
		minStep:     0.5,
		maxInterval: 10 * time.Second,
		formatter:   defaultProgressFormatter,
	}
}

// WithLogWriter 设置进度日志的输出目标。默认 io.Discard。
func WithLogWriter(w io.Writer) ProgressLogOption {
	return func(c *progressLogConfig) {
		c.w = w
	}
}

// WithMinPercentStep 设置触发日志写入的最小进度百分比变化。默认 0.5。
// 设为 0 表示每次回调都写入。
func WithMinPercentStep(step float64) ProgressLogOption {
	return func(c *progressLogConfig) {
		if step < 0 {
			step = 0
		}
		c.minStep = step
	}
}

// WithMaxInterval 设置两次日志写入之间的最大间隔。默认 10s。
// 即使进度变化未达到最小步长，超过此间隔也会强制写入。
func WithMaxInterval(d time.Duration) ProgressLogOption {
	return func(c *progressLogConfig) {
		if d <= 0 {
			d = time.Second
		}
		c.maxInterval = d
	}
}

// WithProgressFormatter 设置自定义日志格式化函数。默认使用 defaultProgressFormatter。
func WithProgressFormatter(fn func(io.Writer, ProgressReport)) ProgressLogOption {
	return func(c *progressLogConfig) {
		c.formatter = fn
	}
}

// NewProgressLogCallback 创建一个带节流的进度日志回调。
//
// 返回的函数签名与 Request.OnProgress 一致，可直接使用或通过 ComposeProgress 与现有回调组合。
// 首次调用无条件写入起点日志；后续调用在进度变化超过 minStep 或距离上次写入超过 maxInterval 时写入。
//
// 使用方法：
//
//	req.OnProgress = NewProgressLogCallback(
//	    WithLogWriter(logFile),
//	    WithMinPercentStep(0.5),
//	    WithMaxInterval(10*time.Second),
//	)
func NewProgressLogCallback(opts ...ProgressLogOption) func(float64, int64, int64) {
	cfg := defaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	var (
		initialTime   = time.Now()
		lastTime      = initialTime
		lastProgress  = -1.0 // 确保第一次调用一定写入
		lastBytes     int64
		firstCallDone bool
		mu            sync.Mutex
	)

	return func(progress float64, downloaded, total int64) {
		if cfg.w == nil {
			return
		}

		now := time.Now()

		mu.Lock()

		// 计算速度（保护除零）
		var speed float64
		elapsed := now.Sub(lastTime).Seconds()
		if elapsed > 0 {
			speed = float64(downloaded-lastBytes) / elapsed
		}

		report := ProgressReport{
			Timestamp:  now,
			Progress:   progress,
			Downloaded: downloaded,
			Total:      total,
			Speed:      speed,
			Elapsed:    now.Sub(initialTime),
		}
		if speed > 0 && total > downloaded {
			report.ETA = time.Duration(float64(total-downloaded)/speed) * time.Second
		}

		// 首次调用或满足节流条件时写入
		deltaPct := progress - lastProgress
		timeSinceLastWrite := now.Sub(lastTime)

		if !firstCallDone || deltaPct >= cfg.minStep || timeSinceLastWrite >= cfg.maxInterval {
			mu.Unlock()
			cfg.formatter(cfg.w, report)
			mu.Lock()
			firstCallDone = true
			lastTime = now
			lastProgress = progress
			lastBytes = downloaded
		}

		mu.Unlock()
	}
}

// defaultProgressFormatter 是默认的进度日志格式化函数。
// 输出格式：
//
//	2026-06-10T12:00:01.000000000Z Progress:  0.000%  0.00 B/s expected time: --.- s
func defaultProgressFormatter(w io.Writer, r ProgressReport) {
	// 时间戳
	ts := r.Timestamp.Format(time.RFC3339Nano)

	// 百分比：7 字符宽右对齐（如 "  0.000%", "100.000%"）
	pctStr := fmt.Sprintf("%7.3f%%", r.Progress)

	// 速度单位自适应
	speedVal := r.Speed
	unit := "B/s"
	switch {
	case speedVal >= 1<<30:
		speedVal /= 1 << 30
		unit = "GB/s"
	case speedVal >= 1<<20:
		speedVal /= 1 << 20
		unit = "MB/s"
	case speedVal >= 1<<10:
		speedVal /= 1 << 10
		unit = "KB/s"
	}

	// ETA
	var etaStr string
	if r.Speed > 0 && r.Total > r.Downloaded {
		etaSec := float64(r.Total-r.Downloaded) / r.Speed
		etaStr = fmt.Sprintf("%5.2f s", etaSec)
	} else {
		etaStr = "--.- s"
	}

	_, err := fmt.Fprintf(w, "%s Progress: %s  %.2f %s expected time: %s\n",
		ts, pctStr, speedVal, unit, etaStr)
	if err != nil {
		slog.Warn("Failed to write progress log", logutil.LogKeyError, err)
	}
}
