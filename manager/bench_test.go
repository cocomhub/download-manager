// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
)

func BenchmarkNewManager(b *testing.B) {
	cfg := &config.Config{
		Server: config.Server{WorkDir: b.TempDir()},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := NewManager(cfg)
		_ = m
	}
}
