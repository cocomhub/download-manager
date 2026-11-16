// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type migrationAuditEntry struct {
	TaskID    string    `json:"task_id"`
	Time      time.Time `json:"time"`
	ObjectURL string    `json:"object_url"`
	Path      string    `json:"path"`
	OldRemote string    `json:"old_remote"`
	NewRemote string    `json:"new_remote"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

func (s *Service) appendMigrationAudit(taskID string, results []MigrationResult) {
	if s.opts.WorkDir == "" || len(results) == 0 {
		return
	}
	dir := filepath.Join(s.opts.WorkDir, "remote_save")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.Logger().Warn("remote-save migration audit mkdir failed", "error", err)
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "migration-audit.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		s.Logger().Warn("remote-save migration audit open failed", "error", err)
		return
	}
	defer f.Close()
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	enc := json.NewEncoder(f)
	for _, res := range results {
		_ = enc.Encode(migrationAuditEntry{
			TaskID: taskID, Time: time.Now().UTC(), ObjectURL: res.ObjectURL, Path: res.Path,
			OldRemote: res.OldRemote, NewRemote: res.NewRemote, Success: res.Success, Error: res.Error,
		})
	}
}
