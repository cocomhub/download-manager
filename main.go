package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"download-manager/api"
	"download-manager/config"
	"download-manager/logutil"
	"download-manager/manager"
)

var (
	configFile  = flag.String("config", "config.yaml", "Path to configuration file")
	versionFlag = flag.Bool("version", false, "Print version and build info")
)

var (
	Version = "dev"
	BuildAt = "unknown"
)

func init() {
	flag.Parse()
}

func main() {
	if *versionFlag {
		fmt.Printf("Version: %s, Build At: %s\n", Version, BuildAt)
		os.Exit(0)
	}

	cfg, err := config.Init(*configFile)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize Logger
	logutil.InitLogger(cfg.Log)

	// Ensure single instance with file lock
	lockFile := cfg.Server.LockFile
	if lockFile == "" {
		lockFile = "download-manager.lock" // Default
	}

	lockFd, err := syscall.Open(lockFile, syscall.O_CREAT|syscall.O_RDWR, 0666)
	if err != nil {
		slog.Error("Failed to open lock file", "file", lockFile, "error", err)
		os.Exit(1)
	}
	defer syscall.Close(lockFd)

	if err := syscall.Flock(lockFd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		slog.Error("Another instance is running", "lock", lockFile)
		os.Exit(1)
	}
	defer syscall.Flock(lockFd, syscall.LOCK_UN)

	mgr := manager.NewManager(cfg)

	// Start Manager
	go mgr.Start()

	go func() {
		// Start HTTP Server
		server := api.NewServer(mgr)
		router := server.Router()

		port := cfg.Server.HTTPPort
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
