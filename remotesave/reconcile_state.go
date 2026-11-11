// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package remotesave

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// deepStatePath 保存最近一次 deep reconcile 时间；进程重启后不会把 24h 计时清零。
func (s *Service) deepStatePath() string {
	if s.opts.WorkDir == "" {
		return ""
	}
	return filepath.Join(s.opts.WorkDir, "remote_save", "deep.state")
}

func (s *Service) loadLastDeep() time.Time {
	p := s.deepStatePath()
	if p == "" {
		return time.Time{}
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil || sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

func (s *Service) saveLastDeep(t time.Time) {
	p := s.deepStatePath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.FormatInt(t.Unix(), 10)), 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, p)
}
