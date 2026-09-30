// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package logutil

import (
	"io"
	"log/slog"
	"os"
	"slices"

	"gopkg.in/natefinch/lumberjack.v2"
)

// ModuleLogConfig 独立模块日志配置（追加式 + 自动轮转）。
type ModuleLogConfig struct {
	Filename   string
	Level      slog.Level
	MaxSize    int // MB
	MaxBackups int
	MaxAge     int // days
	Compress   bool
	Console    bool
}

// ClosableLogger 包装 slog.Logger + 底层文件句柄（可 Close 释放）。
type ClosableLogger struct {
	*slog.Logger
	closers []io.Closer
}

// Close 关闭底层文件句柄（测试/优雅停机释放）。
func (c *ClosableLogger) Close() error {
	for _, v := range slices.Backward(c.closers) {
		v.Close()
	}
	return nil
}

// NewModuleLogger 创建独立的模块日志记录器（与全局默认日志隔离）。
// 底层用 lumberjack：追加写 + 按大小轮转 + 保留备份 + 压缩，适合长期运行。
// 返回的 *slog.Logger 独立使用；组件可直接持有多输出（console + file）。
func NewModuleLogger(cfg ModuleLogConfig) *ClosableLogger {
	var writers []io.Writer
	var closers []io.Closer
	if cfg.Console {
		writers = append(writers, os.Stdout)
	}
	if cfg.Filename != "" {
		lj := &lumberjack.Logger{
			Filename:   cfg.Filename,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}
		writers = append(writers, lj)
		closers = append(closers, lj)
	}
	var w io.Writer
	if len(writers) > 0 {
		w = io.MultiWriter(writers...)
	} else {
		w = io.Discard
	}
	level := cfg.Level
	if level == 0 {
		level = slog.LevelInfo
	}
	return &ClosableLogger{
		Logger: slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
			AddSource: true,
			Level:     level,
		})),
		closers: closers,
	}
}
