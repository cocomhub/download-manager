package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/natefinch/lumberjack.v2"
	"gopkg.in/yaml.v3"

	"download-manager/api"
	"download-manager/config"
	"download-manager/manager"
)

func initLogger(cfg config.LogConfig) {
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
			MaxSize:    cfg.MaxSize, // megabytes
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge, // days
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

func main() {
	configFile := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Read config first to get lock file path
	data, err := os.ReadFile(*configFile)
	if err != nil {
		fmt.Printf("Error reading config file: %v\n", err)
		os.Exit(1)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Printf("Error parsing config file: %v\n", err)
		os.Exit(1)
	}

	// Initialize Logger
	initLogger(cfg.Log)

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

	mgr := manager.NewManager(&cfg)

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

		slog.Info("Starting HTTP server", "port", port)
		slog.Info("Web UI available", "url", fmt.Sprintf("http://localhost:%d", port))
		if err := http.ListenAndServe(fmt.Sprintf(":%d", port), router); err != nil {
			slog.Error("HTTP server failed", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	mgr.Stop()
	slog.Info("Server exited")
}
