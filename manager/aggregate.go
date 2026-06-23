// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"log/slog"
	"maps"
	"strings"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/titlegroup"
	"github.com/cocomhub/download-manager/storage"
)

func (m *Manager) AggregateObjects(page, limit int64, search, sortBy, status string, types []string) (map[string]any, error) {
	return m.aggSvc.AggregateObjects(page, limit, search, sortBy, status, types)
}

// typeMatchesTask checks if the given task type matches any of the given type prefixes.
// It is a package-level helper shared by Manager.AggregateByContent and AggregationService.AggregateObjects.
func typeMatchesTask(t core.Task, types []string) bool {
	if len(types) == 0 {
		return true
	}
	tt := strings.ToLower(t.Type())
	for _, pref := range types {
		p := strings.ToLower(pref)
		if strings.HasPrefix(tt, p) {
			return true
		}
	}
	return false
}

// AggregateByContent groups objects by scoped content group and returns representatives.
func (m *Manager) AggregateByContent(page, limit int64, search, sortBy, status string, types []string) (map[string]any, error) {
	cfg := m.currentCfg()
	type taskObj struct {
		t   core.Task
		obj *model.DownloadObject
	}

	// Collect matching tasks
	var matchingTasks []core.Task
	for _, tCfg := range cfg.Tasks {
		id := tCfg.ID
		tk, ok := m.getTask(id)
		if !ok {
			continue
		}
		if !typeMatchesTask(tk, types) {
			continue
		}
		matchingTasks = append(matchingTasks, tk)
	}

	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 50
	}

	// For content grouping we need ALL matching objects to build groups properly,
	// but we collect each task via Search with search/status filter to reduce data.
	all := make([]taskObj, 0, 1024)
	for _, tk := range matchingTasks {
		query := &core.StorageQuery{
			Filter: core.StorageFilter{
				Search: search,
			},
		}
		if status != "" && status != "all" {
			query.Filter.Statuses = []string{status}
		}
		objs, err := m.collectTaskObjects(tk, query, 200)
		if err != nil {
			return nil, err
		}
		for _, o := range objs {
			all = append(all, taskObj{t: tk, obj: o})
		}
	}
	// Group by task_id + task_type + content_group to avoid cross-task leakage.
	type groupEntry struct {
		t   core.Task
		obj *model.DownloadObject
	}
	groups := make(map[string][]groupEntry)
	for _, to := range all {
		key := scopedContentGroupKey(to.t.ID(), to.t.Type(), metadataContentGroup(to.obj))
		groups[key] = append(groups[key], groupEntry(to))
	}
	// Pick representative by priority, tie -> first.
	reps := make([]*model.DownloadObject, 0, len(groups))
	for _, entries := range groups {
		var rep *model.DownloadObject
		repScore := -1
		for idx, e := range entries {
			score := variantPriorityScore(e.t, e.obj)
			if idx == 0 || score > repScore {
				rep = e.obj
				repScore = score
			}
		}
		if rep != nil {
			// shallow copy Extra/Metadata without copying mu
			copyObj := &model.DownloadObject{
				TaskID:   rep.TaskID,
				URL:      rep.URL,
				SavePath: rep.SavePath,
				Status:   rep.GetStatus(),
				Progress: rep.GetProgress(),
			}
			if rep.Metadata != nil {
				copyObj.Metadata = make(map[string]string, len(rep.Metadata))
				maps.Copy(copyObj.Metadata, rep.Metadata)
			}
			copyObj.Extra = make(map[string]any, len(rep.Extra)+1)
			if rep.Extra != nil {
				maps.Copy(copyObj.Extra, rep.Extra)
			}
			copyObj.Extra["group_size"] = len(entries)
			reps = append(reps, copyObj)
		}
	}
	total := int64(len(reps))
	if page < 1 {
		page = 1
	}
	var offset int64
	if limit <= 0 {
		page = 1
		limit = total
	} else {
		offset = (page - 1) * limit
	}
	paged := storage.ApplyQueryToObjects(reps, &core.StorageQuery{
		Sort:   sortRules(sortBy),
		Offset: offset,
		Limit:  limit,
	})
	return map[string]any{
		"objects": paged,
		"total":   total,
		"page":    page,
		"limit":   limit,
	}, nil
}

func metadataContentGroup(obj *model.DownloadObject) string {
	if obj == nil || obj.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(obj.Metadata[model.MetadataKeyContentGroup])
}

func metadataTaskType(obj *model.DownloadObject) string {
	if obj == nil || obj.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(obj.Metadata["task_type"])
}

func scopedContentGroupKey(taskID, taskType, group string) string {
	return strings.TrimSpace(taskID) + "\x00" + strings.TrimSpace(taskType) + "\x00" + strings.TrimSpace(group)
}

func variantPriorityScore(t core.Task, obj *model.DownloadObject) int {
	if t == nil || obj == nil || t.Type() != core.TaskTypeTktube {
		return 0
	}
	hq, c := titlegroup.TKTVariantFlags(obj.Metadata[model.MetadataKeyTitle])
	switch {
	case hq && c:
		return 4
	case hq:
		return 3
	case c:
		return 2
	default:
		return 1
	}
}

// BackfillContentGroups scans storages and recomputes content_group/task_type metadata for tktube tasks.
func (m *Manager) BackfillContentGroups() {
	m.tasks.Range(func(key, value any) bool {
		t, _ := value.(core.Task)
		if t == nil || t.Type() != core.TaskTypeTktube {
			return true
		}
		st := t.Storage()
		if st == nil {
			return true
		}
		list, err := m.collectTaskObjects(t, &core.StorageQuery{
			Filter: core.StorageFilter{
				TaskIDs: []string{strings.TrimSpace(t.ID())},
			},
		}, 200)
		if err != nil || list == nil {
			return true
		}
		total := 0
		changed := 0
		taskType := strings.TrimSpace(t.Type())
		for _, obj := range list {
			if obj == nil {
				continue
			}
			total++

			obj.Lock()
			if obj.Metadata == nil {
				obj.Metadata = make(map[string]string)
			}
			dirty := false
			newGroup := titlegroup.TKTContentGroupKey(obj.Metadata[model.MetadataKeyTitle], obj.URL)
			if obj.Metadata[model.MetadataKeyContentGroup] != newGroup {
				obj.Metadata[model.MetadataKeyContentGroup] = newGroup
				dirty = true
			}
			if obj.Metadata["task_type"] != taskType {
				obj.Metadata["task_type"] = taskType
				dirty = true
			}
			obj.Unlock()
			if !dirty {
				continue
			}
			if err := st.Update(obj); err != nil {
				slog.Warn("Failed to recompute object metadata", "task_id", t.ID(), "url", obj.URL, "error", err)
				continue
			}
			changed++
		}
		slog.Info("Recomputed object metadata", "task_id", t.ID(), "task_type", t.Type(), "total", total, "changed", changed)
		return true
	})
}

// GetObjectsByScopedGroup returns all objects for the given task_id + task_type + content_group.
func (m *Manager) GetObjectsByScopedGroup(taskID, taskType, group string) []*model.DownloadObject {
	list := make([]*model.DownloadObject, 0, 64)
	taskID = strings.TrimSpace(taskID)
	taskType = strings.TrimSpace(taskType)
	group = strings.TrimSpace(group)
	tk, ok := m.getTask(taskID)
	if !ok || tk.Type() != taskType {
		return list
	}
	objs, err := m.collectTaskObjects(tk, &core.StorageQuery{
		Filter: core.StorageFilter{
			Metadata: map[string]string{"content_group": group},
		},
	}, 200)
	if err == nil {
		list = append(list, objs...)
	}
	return list
}
