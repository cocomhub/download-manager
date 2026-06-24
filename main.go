// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cocomhub/download-manager/api"
	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
	"github.com/cocomhub/download-manager/pkg/logutil"
	"github.com/cocomhub/download-manager/storage"
	"github.com/gofrs/flock"
)

var (
	Version = "dev"
	BuildAt = "unknown"
)

type parseResult struct {
	ConfigPath   string
	ShowVersion  bool
	RunMode      config.RunMode
	RunModeSet   bool
	UsageMessage string
}

func parseFlags(args []string) (parseResult, error) {
	var res parseResult
	var buf bytes.Buffer
	fs := flag.NewFlagSet("download-manager", flag.ContinueOnError)
	fs.SetOutput(&buf)

	var (
		cfgPath string
		showVer bool
		runMode string
		uiOnly  bool
	)

	fs.StringVar(&cfgPath, "config", "config.yaml", "配置文件路径（默认 config.yaml）")
	fs.BoolVar(&showVer, "version", false, "打印版本与构建信息后退出")
	fs.StringVar(&runMode, "run-mode", "", "运行模式：full 或 ui")
	fs.BoolVar(&uiOnly, "ui-only", false, "仅启动 Web UI（等价于 --run-mode=ui）")

	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "用法：download-manager [选项]")
		fmt.Fprintln(fs.Output(), "选项：")
		fs.PrintDefaults()
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "优先级：命令行 > 环境变量 > 配置文件")
		fmt.Fprintln(fs.Output(), "环境变量：")
		fmt.Fprintln(fs.Output(), "  DM_RUN_MODE   设置运行模式 full|ui")
		fmt.Fprintln(fs.Output(), "  DM_UI_ONLY    若为 1/true/yes 则等价 ui 模式")
		fmt.Fprintln(fs.Output(), "  DM_HTTP_PORT  设置 HTTP 端口")
	}

	if err := fs.Parse(args); err != nil {
		res.UsageMessage = buf.String()
		return res, err
	}
	res.UsageMessage = buf.String()
	res.ConfigPath = cfgPath
	res.ShowVersion = showVer

	// Determine effective run-mode with precedence:
	// CLI --run-mode > CLI --ui-only > ENV DM_RUN_MODE > ENV DM_UI_ONLY
	if runMode != "" {
		// CLI --run-mode provided
		res.RunModeSet = true
		switch strings.ToLower(runMode) {
		case "ui":
			res.RunMode = config.RunModeUI
		default:
			res.RunMode = config.RunModeFull
		}
	} else if uiOnly {
		// CLI --ui-only provided
		res.RunModeSet = true
		res.RunMode = config.RunModeUI
	} else if envMode := os.Getenv("DM_RUN_MODE"); envMode != "" {
		// ENV DM_RUN_MODE
		res.RunModeSet = true
		switch strings.ToLower(envMode) {
		case "full":
			res.RunMode = config.RunModeFull
		case "ui":
			res.RunMode = config.RunModeUI
		default:
			res.RunMode = config.RunModeFull
		}
	} else if v, ok := os.LookupEnv("DM_UI_ONLY"); ok && parseBoolEnv(v) {
		// ENV DM_UI_ONLY
		res.RunModeSet = true
		res.RunMode = config.RunModeUI
	}

	return res, nil
}

func parseBoolEnv(v string) bool {
	return v == "1" || v == "true" || v == "TRUE" || v == "True" || v == "yes" || v == "Y" || v == "y"
}

func main() {
	res, err := parseFlags(os.Args[1:])
	if err == flag.ErrHelp {
		fmt.Print(res.UsageMessage)
		return
	} else if err != nil {
		fmt.Fprint(os.Stderr, res.UsageMessage)
		os.Exit(2)
	}

	if res.ShowVersion {
		fmt.Printf("Version: %s, Build At: %s\n", Version, BuildAt)
		os.Exit(0)
	}

	cfg, err := config.Init(res.ConfigPath)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Merge runtime mode with precedence (CLI > Env > Config)
	if res.RunModeSet {
		cfg.Runtime.Mode = res.RunMode
	}

	// Initialize Logger
	logutil.InitLogger(cfg.Log)

	slog.Info("runtime mode", "mode", cfg.Runtime.Mode, "download", cfg.Runtime.Download.Enabled, "scheduler", cfg.Runtime.Scheduler.Enabled)

	// Ensure single instance with file lock
	lockFile := cfg.Server.LockFile
	if cfg.Runtime.Mode == config.RunModeUI {
		lockFile = cfg.Server.UIOnlyLockFile
	}
	if lockFile == "" {
		lockFile = "download-manager.lock" // Default
	}

	fl := flock.New(lockFile)
	locked, err := fl.TryLock()
	if err != nil {
		slog.Error("Failed to acquire lock", "lock", lockFile, logutil.LogKeyError, err)
		os.Exit(1)
	}
	if !locked {
		slog.Error("Another instance is running", "lock", lockFile)
		os.Exit(1)
	}
	defer func() {
		if err := fl.Unlock(); err != nil {
			slog.Warn("unlock failed", "lock", lockFile, logutil.LogKeyError, err)
		}
		_ = fl.Close()
	}()

	mgr := manager.NewManager(cfg)

	// Start Manager
	go mgr.Start()

	// Start HTTP Server
	server := api.NewServer(mgr)
	router := server.Router()

	port := cfg.Server.HTTPPort
	if cfg.Runtime.Mode == config.RunModeUI {
		port = cfg.Server.UIOnlyPort
	}
	if port == 0 {
		port = 8080 // Default port
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router,
	}

	go func() {
		slog.Info("Starting HTTP server", "port", port, "version", Version, "build_at", BuildAt)
		slog.Info("Web UI available", logutil.LogKeyURL, fmt.Sprintf("http://localhost:%d", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server failed", logutil.LogKeyError, err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("Received shutdown signal", "signal", sig)

	// Create shutdown context with 30s timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Second signal: force exit
	go func() {
		sig2 := <-quit
		slog.Warn("Second signal received, force exiting", "signal", sig2)
		os.Exit(1)
	}()

	// Step 1: Mark in-flight objects and stop manager
	mgr.Stop(shutdownCtx)

	// Step 2: Graceful HTTP shutdown
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", logutil.LogKeyError, err)
	}

	// Step 3: Wait for workers and flush storages
	mgr.WaitForShutdown(shutdownCtx)

	// Step 4: Close Mongo clients
	storage.CloseAllMongoClients()

	slog.Info("Server exited gracefully", "version", Version, "build_at", BuildAt)
}
