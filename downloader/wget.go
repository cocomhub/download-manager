package downloader

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"download-manager/config"
	"download-manager/core"
	"download-manager/model"
)

type WgetDownloader struct {
	logDir string
}

// Ensure WgetDownloader implements core.Downloader
var _ core.Downloader = &WgetDownloader{}

func NewWgetDownloader(cfg config.DownloaderConfig) *WgetDownloader {
	logDir := cfg.LogDir
	if err := os.MkdirAll(logDir, 0755); err != nil {
		slog.Error("Failed to create log directory", "dir", logDir, "error", err)
		logDir = ""
	}
	return &WgetDownloader{
		logDir: logDir,
	}
}

func (d *WgetDownloader) Name() string {
	return "wget"
}

func (d *WgetDownloader) Download(obj *model.DownloadObject) error {
	// Ensure directory exists
	dir := filepath.Dir(obj.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Prepare log file for wget output
	// Save logs in a .logs directory relative to the save path or in the same directory with a suffix
	// Let's use a hidden log file in the same directory for simplicity and proximity
	var f *os.File
	var logFile string
	if d.logDir != "" {
		logFile = filepath.Join(d.logDir, filepath.Base(obj.SavePath)+"."+time.Now().Format("20060102150405")+".wget.log")
		var err error
		f, err = os.Create(logFile)
		if err != nil {
			slog.Warn("Failed to create wget log file", "file", logFile, "error", err)
			// Proceed without logging to file if creation fails
		} else {
			defer f.Close()
		}
	}

	// Build wget command
	// -c: continue getting a partially-downloaded file
	// -O: write documents to file
	// -q: quiet (no output) - or remove for debug
	cmd := exec.Command("wget", "-c", obj.URL, "-O", obj.SavePath)

	// Redirect output to log file if available, otherwise to /dev/null to avoid polluting stdout
	if f != nil {
		cmd.Stdout = f
		cmd.Stderr = f
		slog.Info("Starting download", "downloader", "wget", "url", obj.URL, "path", obj.SavePath, "wget_log", logFile)
	} else {
		// If no log file, discard output to keep main logs clean
		// Or keep stderr? User requested "avoid affecting standard output/error"
		cmd.Stdout = nil
		cmd.Stderr = nil
		slog.Info("Starting download", "downloader", "wget", "url", obj.URL, "path", obj.SavePath)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wget execution failed: %w", err)
	}

	return nil
}
