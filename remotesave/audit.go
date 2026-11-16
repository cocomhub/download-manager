// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type MigrationAuditEntry struct {
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
		_ = enc.Encode(MigrationAuditEntry{
			TaskID: taskID, Time: time.Now().UTC(), ObjectURL: res.ObjectURL, Path: res.Path,
			OldRemote: res.OldRemote, NewRemote: res.NewRemote, Success: res.Success, Error: res.Error,
		})
	}
}

// ListMigrationAudit 返回最近 limit 条迁移审计（按写入顺序，最多 500 条）。
func (s *Service) ListMigrationAudit(taskID string, limit int) ([]MigrationAuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if s.opts.WorkDir == "" {
		return nil, nil
	}
	f, err := os.Open(filepath.Join(s.opts.WorkDir, "remote_save", "migration-audit.jsonl"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	out := make([]MigrationAuditEntry, 0, limit)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e MigrationAuditEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		if taskID != "" && e.TaskID != taskID {
			continue
		}
		out = append(out, e)
		if len(out) > limit {
			out = out[len(out)-limit:]
		}
	}
	return out, sc.Err()
}
