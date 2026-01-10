package downloader

import (
	"bytes"
	"fmt"
	"os/exec"

	"download-manager/config"
)

func Scrape(url string) (string, error) {
	cmd := exec.Command(config.GetServerConfig().ScraperPath, url)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("scraper(%s) failed: %v, stderr: %s", config.GetServerConfig().ScraperPath, err, stderr.String())
	}
	return out.String(), nil
}
