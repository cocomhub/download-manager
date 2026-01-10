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
		HTTPPort:    8080,
		WorkDir:     ".",
		LockFile:    "download-manager.lock",
		ScraperPath: "bin/scraper_get",
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

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

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
