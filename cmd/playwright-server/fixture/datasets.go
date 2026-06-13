// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
)

// loadFull sets up 4 pre-configured tasks for E2E testing.
// Must be called before Manager.Start() so loadTasks picks them up.
func loadFull(mgr *manager.Manager) error {
	cur := mgr.GetConfig()
	cur.Tasks = []config.Task{
		{
			ID:      "test-tktube",
			Type:    "mock",
			SaveDir: cur.Server.WorkDir + "/test-tktube",
			Storage: config.StorageConfig{Type: "memory"},
			Extra: map[string]any{
				"mock_rules": []any{
					map[string]any{
						"url_template": "http://fixture/tktube/video-{n}.mp4",
						"count":        15,
						"status":       "completed",
						"metadata": map[string]any{
							"content_group": "group-a",
							"resolution":    "1080p",
							"label":         "tktube-video",
						},
					},
				},
				"mock_behavior": map[string]any{
					"mode": "always_success",
				},
			},
		},
		{
			ID:      "test-vikacg",
			Type:    "mock",
			SaveDir: cur.Server.WorkDir + "/test-vikacg",
			Storage: config.StorageConfig{Type: "memory"},
			Extra: map[string]any{
				"mock_rules": []any{
					map[string]any{
						"url_template": "http://fixture/vikacg/image-{n}.jpg",
						"count":        8,
						"status":       "completed",
						"metadata": map[string]any{
							"label": "vikacg-image",
						},
					},
				},
				"mock_behavior": map[string]any{
					"mode": "always_success",
				},
			},
		},
		{
			ID:      "test-hanime",
			Type:    "mock",
			SaveDir: cur.Server.WorkDir + "/test-hanime",
			Storage: config.StorageConfig{Type: "memory"},
			Extra: map[string]any{
				"mock_rules": []any{
					map[string]any{
						"url_template": "http://fixture/hanime/video-{n}.mp4",
						"count":        6,
						"status":       "completed",
						"metadata": map[string]any{
							"label": "hanime-video",
						},
					},
				},
				"mock_behavior": map[string]any{
					"mode": "always_success",
				},
			},
		},
		{
			ID:      "test-mixed",
			Type:    "mock",
			SaveDir: cur.Server.WorkDir + "/test-mixed",
			Storage: config.StorageConfig{Type: "memory"},
			Extra: map[string]any{
				"mock_rules": []any{
					map[string]any{
						"url_template": "http://fixture/mixed/file-{n}.bin",
						"count":        12,
						"status":       "pending",
						"metadata": map[string]any{
							"label": "mixed-file",
							"size":  "1MB",
						},
					},
				},
				"mock_behavior": map[string]any{
					"mode":      "random_fail",
					"fail_rate": 0.25,
				},
			},
		},
	}
	return nil
}
