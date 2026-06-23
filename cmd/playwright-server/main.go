// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Command playwright-server starts a download-manager server with in-memory
// storage and optional test fixture data, for use as a subprocess from
// Playwright browser E2E tests.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cocomhub/download-manager/api"
	"github.com/cocomhub/download-manager/cmd/playwright-server/fixture"
	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
	_ "github.com/cocomhub/download-manager/task/mock" // register mock task type
)

func main() {
	port := flag.Int("port", 19199, "HTTP server port")
	fixtureName := flag.String("fixture", "", "Test fixture name to load (e.g. 'full')")
	uiOnly := flag.Bool("ui-only", false, "Start in UI-only mode (read-only)")
	flag.Parse()

	runMode := config.RunModeFull
	if *uiOnly {
		runMode = config.RunModeUI
	}

	workDir := os.TempDir() + "/playwright-test"
	downloadDir := os.TempDir() + "/playwright-downloads"
	_ = os.MkdirAll(workDir, 0750)
	_ = os.MkdirAll(downloadDir, 0750)

	// Minimal config: in-memory storage, no mongo, mock downloader
	cfg := &config.Config{
		Server: config.Server{
			HTTPPort:        *port,
			WorkDir:         workDir,
			DownloadRootDir: downloadDir,
		},
		Runtime: config.Runtime{
			Mode: runMode,
			Download: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{Enabled: !*uiOnly},
			Scheduler: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{Enabled: !*uiOnly},
		},
		Downloader: config.Downloader{
			GlobalConcurrent: 5,
			MaxRetries:       2,
		},
		TaskScan: config.TaskScan{
			Interval: 2, // fast scan for E2E tests
		},
	}

	mgr := manager.NewManager(cfg)

	// Load fixture before Start() so loadTasks picks it up
	if *fixtureName != "" {
		if err := fixture.LoadFixture(mgr, *fixtureName); err != nil {
			slog.Error("failed to load fixture", "name", *fixtureName, "error", err)
			os.Exit(1)
		}
	}

	// Start manager
	go mgr.Start()

	// Start HTTP server
	srv := api.NewServer(mgr)
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: srv.Router(),
	}

	go func() {
		slog.Info("playwright test server starting", "port", *port, "fixture", *fixtureName)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for SIGTERM (from Playwright subprocess kill)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig

	slog.Info("shutting down")
	defer os.RemoveAll(workDir)
	defer os.RemoveAll(downloadDir)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	mgr.Stop(ctx)
}
