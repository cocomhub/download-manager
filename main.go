package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/yaml.v3"

	"download-manager/api"
	"download-manager/config"
	"download-manager/manager"
)

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

	// Ensure single instance with file lock
	lockFile := cfg.Server.LockFile
	if lockFile == "" {
		lockFile = "download-manager.lock" // Default
	}

	lockFd, err := syscall.Open(lockFile, syscall.O_CREAT|syscall.O_RDWR, 0666)
	if err != nil {
		fmt.Printf("Failed to open lock file %s: %v\n", lockFile, err)
		os.Exit(1)
	}
	defer syscall.Close(lockFd)

	if err := syscall.Flock(lockFd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		fmt.Printf("Another instance is running (lock: %s). Exiting.\n", lockFile)
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

		fmt.Printf("Starting HTTP server on :%d\n", port)
		fmt.Printf("Web UI available at http://localhost:%d\n", port)
		if err := http.ListenAndServe(fmt.Sprintf(":%d", port), router); err != nil {
			fmt.Printf("HTTP server failed: %v\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	mgr.Stop()
	fmt.Println("Server exited")
}
