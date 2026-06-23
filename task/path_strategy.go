// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"path/filepath"
	"strings"

	"github.com/cocomhub/download-manager/core"
)

type simplePathStrategy struct {
	mode    string
	baseDir string
}

func NewPathStrategy(mode, baseDir string) core.PathStrategy {
	if mode == "" {
		mode = "first_fixed"
	}
	return &simplePathStrategy{mode: mode, baseDir: baseDir}
}

func (s *simplePathStrategy) Resolve(baseDir string, taskID string, title string, fileType string) (dir string, filename string) {
	switch s.mode {
	case "per_task_dir":
		dir = filepath.Join(s.baseDir, taskID)
	case "unified":
		dir = s.baseDir
	default:
		dir = s.baseDir
	}
	name := strings.ReplaceAll(title, "/", "_")
	video := filepath.Join(dir, name+".mp4")
	image := filepath.Join(dir, name+".jpg")
	return video, image
}
