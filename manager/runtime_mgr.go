// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/pkg/configutil"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

func (m *Manager) worker() {
	m.workerHeartbeat.Store(time.Now()) // initial heartbeat for idle worker health check
	hbTicker := time.NewTicker(3 * time.Second)
	defer hbTicker.Stop()
	for {
		select {
		case req, ok := <-m.downloadQueue:
			if !ok {
				return
			}
			if req != nil {
				m.workerHeartbeat.Store(time.Now())
				m.download(req.task, req.obj)
			}
		case <-hbTicker.C:
			m.workerHeartbeat.Store(time.Now())
		case <-m.stopChan:
			return
		case <-m.workerStop:
			return
		}
	}
}

func (m *Manager) adjustGlobalWorkers(newLimit int) {
	if newLimit <= 0 {
		newLimit = 5
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.workerCount.Load()
	if newLimit > int(current) {
		add := newLimit - int(current)
		slog.Info("Increasing global workers", "from", current, "to", newLimit)
		for range add {
			m.workerWg.Go(m.worker)
		}
		m.workerCount.Store(int64(newLimit))
	} else if newLimit < int(current) {
		remove := int(current) - newLimit
		slog.Info("Decreasing global workers", "from", current, "to", newLimit)
		for range remove {
			m.workerStop <- struct{}{}
		}
		m.workerCount.Store(int64(newLimit))
	}
}

func (m *Manager) applyTaskRuntime(newCfg *config.Config) {
	for _, tCfg := range newCfg.Tasks {
		t, ok := m.getTask(tCfg.ID)
		if !ok {
			continue
		}
		applyConcurrency(t, &tCfg)
		applyRefreshInterval(t, &tCfg)
	}
}

func applyConcurrency(t core.Task, tCfg *config.Task) {
	cfgVal := int(configutil.GetInt64(tCfg.Extra, "max_concurrent", 2))
	if t.Concurrency() == cfgVal {
		return
	}
	if err := t.SetConcurrency(cfgVal); err != nil {
		slog.Warn("SetConcurrency failed", logutil.LogKeyTaskID, tCfg.ID, logutil.LogKeyError, err)
		return
	}
	slog.Info("Task concurrency updated", logutil.LogKeyTaskID, tCfg.ID, "value", cfgVal)
}

func applyRefreshInterval(t core.Task, tCfg *config.Task) {
	cfgVal := int(configutil.GetInt64(tCfg.Extra, "refresh_interval", 3600))
	if t.RefreshInterval() == cfgVal {
		return
	}
	if err := t.SetRefreshInterval(cfgVal); err != nil {
		slog.Warn("SetRefreshInterval failed", logutil.LogKeyTaskID, tCfg.ID, logutil.LogKeyError, err)
		return
	}
	slog.Info("Task refresh interval updated", logutil.LogKeyTaskID, tCfg.ID, "value", cfgVal)
}

func (m *Manager) SetTaskConfig(taskID string, concurrency, refreshInterval *int, scrapeEnabled, downloadEnabled *bool, saveSubDir string, audit *AuditInfo) (map[string]bool, error) {
	t, ok := m.getTask(taskID)
	if !ok {
		return nil, fmt.Errorf("%w", errTaskNotFound)
	}
	result := map[string]bool{"concurrency": false, "refresh_interval": false, "scrape_enabled": false, "download_enabled": false, "save_sub_dir": false}
	if concurrency != nil {
		t.SetConcurrency(*concurrency)
		result["concurrency"] = true
	}
	if refreshInterval != nil {
		t.SetRefreshInterval(*refreshInterval)
		result["refresh_interval"] = true
	}
	if scrapeEnabled != nil {
		result["scrape_enabled"] = true
	}
	if downloadEnabled != nil {
		result["download_enabled"] = true
	}
	if saveSubDir != "" {
		result["save_sub_dir"] = true
	}
	if !result["concurrency"] && !result["refresh_interval"] && !result["scrape_enabled"] && !result["download_enabled"] && !result["save_sub_dir"] {
		return result, nil
	}
	return result, m.saveTaskConfig(taskID, concurrency, refreshInterval, scrapeEnabled, downloadEnabled, saveSubDir, audit)
}

func (m *Manager) saveTaskConfig(taskID string, concurrency, refreshInterval *int, scrapeEnabled, downloadEnabled *bool, saveSubDir string, audit *AuditInfo) error {
	m.mu.Lock()
	cfgCopy := m.currentCfg().Clone()
	for i := range cfgCopy.Tasks {
		if cfgCopy.Tasks[i].ID == taskID {
			updateTaskConfigExtra(&cfgCopy.Tasks[i], concurrency, refreshInterval)
			updateTaskConfigTopLevel(&cfgCopy.Tasks[i], scrapeEnabled, downloadEnabled, saveSubDir)
			break
		}
	}
	m.mu.Unlock()
	return m.UpdateConfig(cfgCopy, audit)
}

func updateTaskConfigExtra(task *config.Task, concurrency, refreshInterval *int) {
	if task.Extra == nil {
		task.Extra = make(map[string]any)
	}
	if concurrency != nil {
		task.Extra["max_concurrent"] = *concurrency
	}
	if refreshInterval != nil {
		task.Extra["refresh_interval"] = *refreshInterval
	}
}

// updateTaskConfigTopLevel writes scrape_enabled, download_enabled, and save_sub_dir
// directly into the config.Task struct fields (not Extra).
func updateTaskConfigTopLevel(task *config.Task, scrapeEnabled, downloadEnabled *bool, saveSubDir string) {
	if scrapeEnabled != nil {
		v := *scrapeEnabled
		task.ScrapeEnabled = &v
	}
	if downloadEnabled != nil {
		v := *downloadEnabled
		task.DownloadEnabled = &v
	}
	if saveSubDir != "" {
		task.SaveSubDir = saveSubDir
	}
}
