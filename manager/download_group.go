// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"log/slog"
	"strings"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

// applyGroupPriorityPolicies enforces group priority within the current tktube task only.
// When multiple objects share the same content_group, the one with the highest priority
// variant (by resolution, subtitle marking, etc.) is kept; lower-priority pending objects
// are auto-cancelled with a redirect_url pointing to the canonical object.
func (m *Manager) applyGroupPriorityPolicies(t core.Task, obj *model.DownloadObject) {
	if t.Type() != core.TaskTypeTktube {
		return
	}
	if obj == nil || obj.GetStatus() != model.StatusCompleted {
		return
	}
	taskType := strings.TrimSpace(t.Type())
	if taskType == "" || metadataTaskType(obj) != taskType {
		return
	}
	group := metadataContentGroup(obj)
	if strings.TrimSpace(group) == "" {
		return
	}
	taskID := strings.TrimSpace(t.ID())
	if taskID == "" || strings.TrimSpace(obj.TaskID) != taskID {
		return
	}
	st := t.Storage()
	if st == nil {
		return
	}
	list, err := m.collectTaskObjects(t, &core.StorageQuery{
		Filter: core.StorageFilter{
			Metadata: map[string]string{"task_type": taskType, "content_group": group},
		},
	}, 200)
	if err != nil || list == nil {
		return
	}
	type candidate struct {
		o     *model.DownloadObject
		score int
	}
	var canonical *model.DownloadObject
	bestScore := -1
	cands := make([]candidate, 0, 8)
	priorityCounts := make(map[int]int, 4)
	for _, o := range list {
		if o == nil {
			continue
		}
		if strings.TrimSpace(o.TaskID) != taskID {
			continue
		}
		if metadataTaskType(o) != taskType {
			continue
		}
		if metadataContentGroup(o) != group {
			continue
		}
		score := variantPriorityScore(t, o)
		cands = append(cands, candidate{o: o, score: score})
		priorityCounts[score]++
		if o.GetStatus() == model.StatusCompleted {
			if canonical == nil || score > bestScore {
				canonical = o
				bestScore = score
			}
		}
	}
	for priority, count := range priorityCounts {
		if count > 1 {
			slog.Info("Skip auto-cancel for conflicting content group priority", logutil.LogKeyTaskID, t.ID(), "task_type", t.Type(), "content_group", group, "priority", priority, "count", count)
			return
		}
	}
	if canonical == nil {
		return
	}
	for _, cnd := range cands {
		o := cnd.o
		if o.URL == canonical.URL {
			continue
		}
		// Auto-cancel only lower-priority pending objects.
		if cnd.score < bestScore && o.GetStatus() == model.StatusPending {
			if o.Extra == nil {
				o.Extra = make(map[string]any)
			}
			o.Extra["redirect_url"] = canonical.URL
			if err := t.UpdateStatus(o, model.StatusCancelled, nil); err != nil {
				slog.Warn("Failed to auto-cancel lower-priority duplicate", logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, o.URL, logutil.LogKeyError, err)
			}
		}
	}
}
