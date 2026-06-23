// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

func BenchmarkValidateAndClamp(b *testing.B) {
	cfg := &Config{
		Server: Server{
			HTTPPort:        8080,
			WorkDir:         "/tmp/work",
			DownloadRootDir: "/tmp/downloads",
		},
		Downloader: Downloader{
			Type:             "native",
			GlobalConcurrent: 5,
			MaxRetries:       3,
		},
		Tasks: []Task{
			{ID: "task1", Type: "tktube", SaveDir: "/tmp/task1",
				Storage: StorageConfig{Type: "file"}},
			{ID: "task2", Type: "hanime", SaveDir: "/tmp/task2",
				Storage: StorageConfig{Type: "file"}},
		},
		TaskScan: TaskScan{Interval: 10},
	}
	b.ResetTimer()
	for b.Loop() {
		cfg.ValidateAndClamp()
	}
}

func BenchmarkClone(b *testing.B) {
	cfg := &Config{
		Server: Server{HTTPPort: 8080, WorkDir: "/tmp/work"},
		Downloader: Downloader{
			Type:             "native",
			GlobalConcurrent: 5,
			Proxies:          []string{"http://proxy1:8080", "http://proxy2:8080"},
			DomainLimits:     map[string]int{"a.com": 3, "b.com": 5, "c.com": 2},
		},
		Tasks: []Task{
			{ID: "task1", Type: "tktube", SaveDir: "/tmp/task1",
				Extra: map[string]any{"key1": "val1", "key2": 42}},
			{ID: "task2", Type: "hanime", SaveDir: "/tmp/task2",
				Extra: map[string]any{"key3": true}},
		},
	}
	b.ResetTimer()
	for b.Loop() {
		_ = cfg.Clone()
	}
}
