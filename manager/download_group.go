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

// groupCandidate pairs a download object with its variant priority score.
type groupCandidate struct {
	o     *model.DownloadObject
	score int
}

// applyGroupPriorityPolicies enforces group priority within the current tktube task only.
// When multiple objects share the same content_group, the one with the highest priority
// variant (by resolution, subtitle marking, etc.) is kept; lower-priority pending objects
// are auto-cancelled with a redirect_url pointing to the canonical object.
func (m *Manager) applyGroupPriorityPolicies(t core.Task, obj *model.DownloadObject) {
	taskType, group, taskID, ok := resolveGroupPriorityParams(t, obj)
	if !ok {
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

	cands, canonical, bestScore, conflicted := processGroupCandidates(list, taskID, taskType, group, t)
	if conflicted || canonical == nil {
		return
	}

	cancelLowerPriorityObjects(t, cands, canonical.URL, bestScore)
}

// resolveGroupPriorityParams validates preconditions and extracts task metadata parameters.
// Returns false when any precondition fails, in which case the policy should be skipped.
func resolveGroupPriorityParams(t core.Task, obj *model.DownloadObject) (taskType, group, taskID string, ok bool) {
	if obj == nil || obj.GetStatus() != model.StatusCompleted {
		return
	}
	taskType = strings.TrimSpace(t.Type())
	if taskType == "" || metadataTaskType(obj) != taskType {
		return
	}
	group = metadataContentGroup(obj)
	if strings.TrimSpace(group) == "" {
		return
	}
	taskID = strings.TrimSpace(t.ID())
	if taskID == "" || strings.TrimSpace(obj.TaskID) != taskID {
		return
	}
	if t.Storage() == nil {
		return
	}
	ok = true
	return
}

// processGroupCandidates filters query results to the matching content group, finds the
// canonical completed object with the highest variant priority, and detects priority conflicts.
// Returns conflicted=true when multiple objects share the same priority score, in which case
// auto-cancellation is skipped to avoid ambiguity.
func processGroupCandidates(list []*model.DownloadObject, taskID, taskType, group string, t core.Task) (
	cands []groupCandidate,
	canonical *model.DownloadObject,
	bestScore int,
	conflicted bool,
) {
	bestScore = -1
	cands = make([]groupCandidate, 0, 8)
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
		cands = append(cands, groupCandidate{o: o, score: score})
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
			slog.Info("Skip auto-cancel for conflicting content group priority",
				logutil.LogKeyTaskID, t.ID(), "task_type", t.Type(),
				"content_group", group, "priority", priority, "count", count)
			conflicted = true
			return
		}
	}
	return
}

// cancelLowerPriorityObjects cancels pending objects whose variant priority is lower than the
// canonical object, setting a redirect_url pointing to the canonical URL.
func cancelLowerPriorityObjects(t core.Task, cands []groupCandidate, canonicalURL string, bestScore int) {
	for _, cnd := range cands {
		o := cnd.o
		if o.URL == canonicalURL {
			continue
		}
		if cnd.score < bestScore && o.GetStatus() == model.StatusPending {
			if o.Extra == nil {
				o.Extra = make(map[string]any)
			}
			o.Extra["redirect_url"] = canonicalURL
			if err := t.UpdateStatus(o, model.StatusCancelled, nil); err != nil {
				slog.Warn("Failed to auto-cancel lower-priority duplicate",
					logutil.LogKeyTaskID, t.ID(), logutil.LogKeyURL, o.URL, logutil.LogKeyError, err)
			}
		}
	}
}
