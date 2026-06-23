// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package logutil

import (
	"io"
	"log/slog"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

type LogConfig struct {
	Level      string `yaml:"level" json:"level"`
	Filename   string `yaml:"filename" json:"filename"`
	MaxSize    int    `yaml:"max_size" json:"max_size"`
	MaxBackups int    `yaml:"max_backups" json:"max_backups"`
	MaxAge     int    `yaml:"max_age" json:"max_age"`
	Console    bool   `yaml:"console" json:"console"`
	Compress   bool   `yaml:"compress" json:"compress"`
}

func InitLogger(cfg LogConfig) {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var writers []io.Writer
	if cfg.Console {
		writers = append(writers, os.Stdout)
	}
	if cfg.Filename != "" {
		fileWriter := &lumberjack.Logger{
			Filename:   cfg.Filename,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}
		writers = append(writers, fileWriter)
	}

	var w io.Writer
	if len(writers) > 0 {
		w = io.MultiWriter(writers...)
	} else {
		w = io.Discard
	}

	logger := slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)
}
