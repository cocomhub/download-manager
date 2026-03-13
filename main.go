// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/cocomhub/download-manager/api"
	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
	"github.com/cocomhub/download-manager/pkg/logutil"
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

func parseBoolEnv(v string) bool {
	return v == "1" || v == "true" || v == "TRUE" || v == "True" || v == "yes" || v == "Y" || v == "y"
}

func parseFlags(args []string, env map[string]string) (parseResult, error) {
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
	fs.StringVar(&runMode, "run-mode", "", "运行模式：full 或 ui。与 --ui-only 同时提供时，优先使用 --run-mode")
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
	}

	if err := fs.Parse(args); err != nil {
		// Capture usage on error (including -h)
		res.UsageMessage = buf.String()
		return res, err
	}
	res.UsageMessage = buf.String()
	res.ConfigPath = cfgPath
	res.ShowVersion = showVer

	// Determine effective run-mode with precedence:
	// CLI --run-mode > CLI --ui-only > ENV DM_RUN_MODE > ENV DM_UI_ONLY
	var (
		modeProvided bool
		modeValue    string
	)
	if runMode != "" {
		modeProvided = true
		modeValue = runMode
	} else if uiOnly {
		modeProvided = true
		modeValue = "ui"
	} else if v, ok := env["DM_RUN_MODE"]; ok && v != "" {
		modeProvided = true
		modeValue = v
	} else if v, ok := env["DM_UI_ONLY"]; ok && parseBoolEnv(v) {
		modeProvided = true
		modeValue = "ui"
	}

	if modeProvided {
		switch modeValue {
		case string(config.RunModeFull), "FULL", "Full":
			res.RunMode = config.RunModeFull
			res.RunModeSet = true
		case string(config.RunModeUI), "UI", "Ui":
			res.RunMode = config.RunModeUI
			res.RunModeSet = true
		default:
			// illegal value fallback to full
			res.RunMode = config.RunModeFull
			res.RunModeSet = true
		}
	}
	return res, nil
}

func main() {
	// build env map
	env := map[string]string{}
	for _, kv := range os.Environ() {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				env[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	res, err := parseFlags(os.Args[1:], env)
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

	// Merge runtime mode with precedence
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
		slog.Error("Failed to acquire lock", "lock", lockFile, "error", err)
		os.Exit(1)
	}
	if !locked {
		slog.Error("Another instance is running", "lock", lockFile)
		os.Exit(1)
	}
	defer func() {
		if err := fl.Unlock(); err != nil {
			slog.Warn("unlock failed", "lock", lockFile, "error", err)
		}
		_ = fl.Close()
	}()

	mgr := manager.NewManager(cfg)

	// Start Manager
	go mgr.Start()

	go func() {
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

		slog.Info("Starting HTTP server", "port", port, "version", Version, "build_at", BuildAt)
		slog.Info("Web UI available", "url", fmt.Sprintf("http://localhost:%d", port))
		if err := http.ListenAndServe(fmt.Sprintf(":%d", port), router); err != nil {
			slog.Error("HTTP server failed", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	mgr.Stop()
	slog.Info("Server exited", "version", Version, "build_at", BuildAt)
}
