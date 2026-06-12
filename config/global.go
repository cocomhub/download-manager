// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/spf13/viper"
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

func SetServerConfig(s Server) {
	serverConfig.Store(s)
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

// Load reads a YAML config file, applies defaults, env overrides via Viper,
// and returns the parsed Config.
//
// Priority (lowest to highest):
//  1. Built-in defaults (from defaultConfig + ValidateAndClamp)
//  2. YAML config file
//  3. Environment variables (DM_* prefix)
//
// Phase 1 migration: Viper is used for env variable binding only.
// The Config struct is still unmarshalled via yaml.Unmarshal to avoid
// requiring mapstructure tags on every field.
func Load(configFile string) (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Apply environment variable overrides via Viper
	applyViperEnvOverrides(&cfg, configFile)

	cfg.ValidateAndClamp()
	return &cfg, nil
}

// Save marshals a Config to YAML and writes it to the file.
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

// defaultConfig returns a Config with built-in defaults.
func defaultConfig() Config {
	return Config{
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
}

// applyViperEnvOverrides sets up Viper for DM_* env vars and overrides
// corresponding fields in the Config. This centralizes env handling that
// was previously spread across main.go parseFlags and manual env lookup.
//
// Currently supported env vars:
//   - DM_RUN_MODE   → runtime.mode
//   - DM_HTTP_PORT  → server.http_port
//   - DM_UI_ONLY    → sets runtime.mode=ui (legacy)
func applyViperEnvOverrides(cfg *Config, configFile string) {
	v := viper.New()
	v.SetConfigFile(configFile)
	v.SetEnvPrefix("DM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Bind known env → config key mappings
	_ = v.BindEnv("runtime.mode", "DM_RUN_MODE")
	_ = v.BindEnv("server.http_port", "DM_HTTP_PORT")

	// DM_RUN_MODE
	if v := os.Getenv("DM_RUN_MODE"); v != "" {
		switch strings.ToLower(v) {
		case "ui":
			cfg.Runtime.Mode = RunModeUI
		case "full":
			cfg.Runtime.Mode = RunModeFull
		}
	}

	// DM_HTTP_PORT
	if v := os.Getenv("DM_HTTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.HTTPPort = port
		}
	}

	// Legacy DM_UI_ONLY
	uiOnly := os.Getenv("DM_UI_ONLY")
	if uiOnly != "" && !v.IsSet("runtime.mode") && os.Getenv("DM_RUN_MODE") == "" {
		switch uiOnly {
		case "1", "true", "TRUE", "True", "yes", "Y", "y":
			cfg.Runtime.Mode = RunModeUI
		}
	}
}
