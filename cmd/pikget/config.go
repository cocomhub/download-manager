// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// config 是 pikget 配置（config.yaml）。
type config struct {
	Downloader downloaderConfig `yaml:"downloader"`
}

type downloaderConfig struct {
	HTTP   httpConfig `yaml:"http"`
	Pikpak pikpakCfg  `yaml:"pikpak"`
}

type httpConfig struct {
	UserAgent   string   `yaml:"user_agent"`
	Proxy       string   `yaml:"proxy"`
	TimeoutSecs int      `yaml:"timeout_secs"`
	MaxRetries  int      `yaml:"max_retries"`
	Headers     []string `yaml:"headers"`
}

type pikpakCfg struct {
	ShareRatio  float64 `yaml:"share_ratio"`
	ChunkSize   int64   `yaml:"chunk_size"`
	Concurrency int     `yaml:"concurrency"`
	// DisableAutoRemove 默认 false = 结束自动永久删除转存副本（安全默认）。
	// 配置纪律：bool 必须默认 false——用**反转语义**（disable_auto_remove）表达
	// 「默认安全行为」，而不是默认 true。
	DisableAutoRemove bool   `yaml:"disable_auto_remove"`
	AccountsDir       string `yaml:"accounts_dir"`
	SecretsDir        string `yaml:"secrets_dir"`
}

// defaultConfig 返回默认配置。
func defaultConfig() config {
	return config{
		Downloader: downloaderConfig{
			HTTP: httpConfig{
				UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/117.0.0.0 Safari/537.36",
				TimeoutSecs: 300,
				MaxRetries:  3,
			},
			Pikpak: pikpakCfg{
				ShareRatio:  0.5,
				ChunkSize:   64 << 20, // 64MiB
				Concurrency: 4,
				// DisableAutoRemove 默认 false → hybrid AutoDelete=true（结束自动删转存）
				DisableAutoRemove: false,
			},
		},
	}
}

// defaultConfigPath 返回默认配置文件路径 ~/.config/pikget/config.yaml。
func defaultConfigPath() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", "config.yaml")
	}
	return filepath.Join(cfgDir, "pikget", "config.yaml")
}

// loadConfig 加载 config.yaml；文件不存在返回默认配置。
func loadConfig(path string) (config, error) {
	cfg := defaultConfig()
	if path == "" {
		path = defaultConfigPath()
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // 无配置文件用默认
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
