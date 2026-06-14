// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
)

// datasets are populated during init() below.
// Registry is in registry.go.

func init() {
	datasets["full"] = loadFull
	datasets["cancel-test"] = loadCancelTest
	datasets["group-test"] = loadGroupTest
	datasets["empty-task"] = loadEmptyTask
	datasets["large-task"] = loadLargeTask
}

func workDir(mgr *manager.Manager) string {
	return mgr.GetConfig().Server.WorkDir
}

// loadFull: 4 default tasks — 41 objects total
func loadFull(mgr *manager.Manager) error {
	cur := mgr.GetConfig()
	cur.Tasks = []config.Task{
		{
			ID:      "test-tktube",
			Type:    "mock",
			SaveDir: workDir(mgr) + "/test-tktube",
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
			SaveDir: workDir(mgr) + "/test-vikacg",
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
			SaveDir: workDir(mgr) + "/test-hanime",
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
			SaveDir: workDir(mgr) + "/test-mixed",
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

// loadCancelTest: 1 task with 20 objects all in "pending" state
func loadCancelTest(mgr *manager.Manager) error {
	cur := mgr.GetConfig()
	cur.Tasks = []config.Task{
		{
			ID:      "cancel-stress",
			Type:    "mock",
			SaveDir: workDir(mgr) + "/cancel-stress",
			Storage: config.StorageConfig{Type: "memory"},
			Extra: map[string]any{
				"mock_rules": []any{
					map[string]any{
						"url_template": "http://fixture/cancel/obj-{n}.dat",
						"count":        20,
						"status":       "pending",
						"metadata": map[string]any{
							"label": "cancel-target",
						},
					},
				},
				"mock_behavior": map[string]any{
					"mode":      "simulate_progress",
					"fail_rate": 0.1,
				},
			},
		},
	}
	return nil
}

// loadGroupTest: 2 tasks with content_group metadata
func loadGroupTest(mgr *manager.Manager) error {
	cur := mgr.GetConfig()
	cur.Tasks = []config.Task{
		{
			ID:      "group-a",
			Type:    "mock",
			SaveDir: workDir(mgr) + "/group-a",
			Storage: config.StorageConfig{Type: "memory"},
			Extra: map[string]any{
				"mock_rules": []any{
					map[string]any{
						"url_template": "http://fixture/groups/group-a/video-{n}.mp4",
						"count":        5,
						"status":       "completed",
						"metadata": map[string]any{
							"content_group": "show-alpha",
							"resolution":    "720p",
						},
					},
					map[string]any{
						"url_template": "http://fixture/groups/group-a/image-{n}.jpg",
						"count":        3,
						"status":       "pending",
						"metadata": map[string]any{
							"content_group": "show-alpha",
							"label":         "cover",
						},
					},
				},
				"mock_behavior": map[string]any{
					"mode": "always_success",
				},
			},
		},
		{
			ID:      "group-b",
			Type:    "mock",
			SaveDir: workDir(mgr) + "/group-b",
			Storage: config.StorageConfig{Type: "memory"},
			Extra: map[string]any{
				"mock_rules": []any{
					map[string]any{
						"url_template": "http://fixture/groups/group-b/video-{n}.mp4",
						"count":        3,
						"status":       "completed",
						"metadata": map[string]any{
							"content_group": "show-beta",
							"resolution":    "1080p",
						},
					},
				},
				"mock_behavior": map[string]any{
					"mode": "always_success",
				},
			},
		},
	}
	return nil
}

// loadEmptyTask: 1 task with 0 objects
func loadEmptyTask(mgr *manager.Manager) error {
	cur := mgr.GetConfig()
	cur.Tasks = []config.Task{
		{
			ID:      "empty-task",
			Type:    "mock",
			SaveDir: workDir(mgr) + "/empty",
			Storage: config.StorageConfig{Type: "memory"},
			Extra: map[string]any{
				"mock_behavior": map[string]any{
					"mode": "always_success",
				},
			},
		},
	}
	return nil
}

// loadLargeTask: 1 task with 100 objects
func loadLargeTask(mgr *manager.Manager) error {
	cur := mgr.GetConfig()
	cur.Tasks = []config.Task{
		{
			ID:      "large-task",
			Type:    "mock",
			SaveDir: workDir(mgr) + "/large",
			Storage: config.StorageConfig{Type: "memory"},
			Extra: map[string]any{
				"mock_rules": []any{
					map[string]any{
						"url_template": "http://fixture/large/file-{n}.bin",
						"count":        100,
						"status":       "completed",
						"metadata": map[string]any{
							"label": "large-dataset",
						},
					},
				},
				"mock_behavior": map[string]any{
					"mode": "always_success",
				},
			},
		},
	}
	return nil
}
