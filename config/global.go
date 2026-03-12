// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

var (
	serverConfig   atomic.Value
	configFilePath atomic.Value
)

func init() {
	SetConfigFilePath("config.yaml")
	SetServerConfig(Server{
		HTTPPort:       8080,
		UIOnlyPort:     8081,
		WorkDir:        ".",
		LockFile:       "download-manager.lock",
		UIOnlyLockFile: "download-manager-ui.lock",
		ScraperPath:    "bin/scraper_get",
	})
}

func SetConfigFilePath(path string) {
	configFilePath.Store(path)
}

func GetConfigFilePath() string {
	return configFilePath.Load().(string)
}

func SetServerConfig(config Server) {
	serverConfig.Store(config)
}

func GetServerConfig() Server {
	return serverConfig.Load().(Server)
}

func GetWorkDir() string {
	if GetServerConfig().WorkDir == "" {
		return "."
	}
	return GetServerConfig().WorkDir
}

func Init(configFile string) (*Config, error) {
	cfg, err := Load(configFile)
	if err != nil {
		return nil, err
	}
	SetConfigFilePath(configFile)
	SetServerConfig(cfg.Server)
	return cfg, nil
}

func Load(configFile string) (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Pre-initialize defaults for sections that require non-zero defaults
	cfg := Config{
		Runtime: Runtime{
			Mode: RunModeFull,
			Download: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{Enabled: true},
			Scheduler: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{Enabled: true},
		},
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	cfg.ValidateAndClamp()
	return &cfg, nil
}

func Save(configFile string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}
