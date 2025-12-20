package downloader

import (
	"download-manager/core"
	"download-manager/model"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type WgetDownloader struct{}

// Ensure WgetDownloader implements core.Downloader
var _ core.Downloader = &WgetDownloader{}

func NewWgetDownloader() *WgetDownloader {
	return &WgetDownloader{}
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

	// Build wget command
	// -c: continue getting a partially-downloaded file
	// -O: write documents to file
	// -q: quiet (no output) - or remove for debug
	cmd := exec.Command("wget", "-c", obj.URL, "-O", obj.SavePath)

	// Redirect output to stdout/stderr for visibility during dev
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("[Wget] Starting download: %s -> %s\n", obj.URL, obj.SavePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wget execution failed: %w", err)
	}

	return nil
}
