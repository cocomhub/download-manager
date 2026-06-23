// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func FuzzValidateAndClamp(f *testing.F) {
	// Seed corpus: structurally valid YAML snippets
	seeds := []string{
		"server:\n  http_port: 8080\n",
		"downloader:\n  type: native\n  global_concurrent: 5\n",
		"tasks:\n  - id: test\n    type: tktube\n    url: http://example.com\n",
		"runtime:\n  mode: full\n",
		"storage:\n  type: file\n  path: ./data\n",
		"contexts:\n  ctx1:\n    storage:\n      type: file\n      path: ./ctx\n",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return // Invalid YAML is not what we're testing
		}
		// ValidateAndClamp should never panic, even on partially populated Config
		cfg.ValidateAndClamp()
	})
}
