// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"log/slog"
	"maps"
	"strings"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/logutil"
	"github.com/cocomhub/download-manager/pkg/titlegroup"
	"github.com/cocomhub/download-manager/storage"
)

func (m *Manager) AggregateObjects(page, limit int64, search, sortBy, status string, types []string) (map[string]any, error) {
	return m.aggSvc.AggregateObjects(page, limit, search, sortBy, status, types)
}

// typeMatchesTask checks if the given task type matches any of the given type prefixes.
func typeMatchesTask(t core.Task, types []string) bool {
	if len(types) == 0 {
		return true
	}
	tt := strings.ToLower(t.Type())
	for _, pref := range types {
		if strings.HasPrefix(tt, strings.ToLower(pref)) {
			return true
		}
	}
	return false
}

// contentGroupEntry associates a task with one of its download objects during aggregation.
type contentGroupEntry struct {
	task core.Task
	obj  *model.DownloadObject
}

// collectMatchingTasks returns registered tasks whose types match the given type prefixes.
func collectMatchingTasks(cfg *config.Config, getTask func(string) (core.Task, bool), types []string) []core.Task {
	var matching []core.Task
	for _, tCfg := range cfg.Tasks {
		tk, ok := getTask(tCfg.ID)
		if !ok {
			continue
		}
		if !typeMatchesTask(tk, types) {
			continue
		}
		matching = append(matching, tk)
	}
	return matching
}

// buildContentQuery constructs a StorageQuery filtered by the given search text and (optionally) status.
func buildContentQuery(search, status string) *core.StorageQuery {
	q := &core.StorageQuery{
		Filter: core.StorageFilter{Search: search},
	}
	if status != "" && status != "all" {
		q.Filter.Statuses = []string{status}
	}
	return q
}

// collectContentObjects gathers all matching objects from the given tasks.
func collectContentObjects(m *Manager, tasks []core.Task, search, status string) ([]contentGroupEntry, error) {
	all := make([]contentGroupEntry, 0, 1024)
	for _, tk := range tasks {
		objs, err := m.collectTaskObjects(tk, buildContentQuery(search, status), 200)
		if err != nil {
			return nil, err
		}
		for _, o := range objs {
			all = append(all, contentGroupEntry{task: tk, obj: o})
		}
	}
	return all, nil
}

// groupByContentKey partitions entries by scoped content group key (task_id + task_type + content_group).
func groupByContentKey(entries []contentGroupEntry) map[string][]contentGroupEntry {
	groups := make(map[string][]contentGroupEntry)
	for _, e := range entries {
		key := scopedContentGroupKey(e.task.ID(), e.task.Type(), metadataContentGroup(e.obj))
		groups[key] = append(groups[key], e)
	}
	return groups
}

// pickRepresentative selects the best object within a group by variant priority (tie goes to first).
func pickRepresentative(entries []contentGroupEntry) *model.DownloadObject {
	var rep *model.DownloadObject
	bestScore := -1
	for idx, e := range entries {
		score := variantPriorityScore(e.task, e.obj)
		if idx == 0 || score > bestScore {
			rep = e.obj
			bestScore = score
		}
	}
	return rep
}

// copyRepresentative creates a shallow copy of rep and attaches the group_size extra field.
func copyRepresentative(rep *model.DownloadObject, groupSize int) *model.DownloadObject {
	c := &model.DownloadObject{
		TaskID:   rep.TaskID,
		URL:      rep.URL,
		SavePath: rep.SavePath,
		Status:   rep.GetStatus(),
		Progress: rep.GetProgress(),
	}
	if rep.Metadata != nil {
		c.Metadata = make(map[string]string, len(rep.Metadata))
		maps.Copy(c.Metadata, rep.Metadata)
	}
	c.Extra = make(map[string]any, len(rep.Extra)+1)
	if rep.Extra != nil {
		maps.Copy(c.Extra, rep.Extra)
	}
	c.Extra["group_size"] = groupSize
	return c
}

// selectGroupRepresentatives picks one representative per content group and copies it.
func selectGroupRepresentatives(groups map[string][]contentGroupEntry) []*model.DownloadObject {
	reps := make([]*model.DownloadObject, 0, len(groups))
	for _, entries := range groups {
		rep := pickRepresentative(entries)
		if rep == nil {
			continue
		}
		reps = append(reps, copyRepresentative(rep, len(entries)))
	}
	return reps
}

// paginateContentResults applies sorting, offset, and limit to the representatives list.
func paginateContentResults(reps []*model.DownloadObject, page, limit int64, sortBy string) (paged []*model.DownloadObject, total, outPage, outLimit int64) {
	total = int64(len(reps))
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
	paged = storage.ApplyQueryToObjects(reps, &core.StorageQuery{
		Sort:   sortRules(sortBy),
		Offset: offset,
		Limit:  limit,
	})
	return paged, total, page, limit
}

// AggregateByContent groups objects by scoped content group and returns representatives.
func (m *Manager) AggregateByContent(page, limit int64, search, sortBy, status string, types []string) (map[string]any, error) {
	matchingTasks := collectMatchingTasks(m.currentCfg(), m.getTask, types)

	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 50
	}

	all, err := collectContentObjects(m, matchingTasks, search, status)
	if err != nil {
		return nil, err
	}

	groups := groupByContentKey(all)
	reps := selectGroupRepresentatives(groups)
	paged, total, page, limit := paginateContentResults(reps, page, limit, sortBy)

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
		m.processOneBackfillTask(value)
		return true
	})
}

// processOneBackfillTask processes a single value from the tasks map during backfill.
func (m *Manager) processOneBackfillTask(value any) bool {
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
	taskType := strings.TrimSpace(t.Type())
	total := 0
	changed := 0
	for _, obj := range list {
		if obj == nil {
			continue
		}
		total++
		if applyBackfillMetadata(obj, taskType, t.ID(), st) {
			changed++
		}
	}
	slog.Info("Recomputed object metadata", logutil.LogKeyTaskID, t.ID(), "task_type", t.Type(), "total", total, "changed", changed)
	return true
}

// applyBackfillMetadata computes content_group and task_type metadata for a tktube object
// and persists it if changed. Returns true if a change was made.
func applyBackfillMetadata(obj *model.DownloadObject, taskType, taskID string, st core.Storage) bool {
	obj.Lock()
	if obj.Metadata == nil {
		obj.Metadata = make(map[string]string)
	}
	newGroup := titlegroup.TKTContentGroupKey(obj.Metadata[model.MetadataKeyTitle], obj.URL)
	groupChanged := obj.Metadata[model.MetadataKeyContentGroup] != newGroup
	typeChanged := obj.Metadata["task_type"] != taskType
	if groupChanged {
		obj.Metadata[model.MetadataKeyContentGroup] = newGroup
	}
	if typeChanged {
		obj.Metadata["task_type"] = taskType
	}
	dirty := groupChanged || typeChanged
	obj.Unlock()

	if !dirty {
		return false
	}
	if err := st.Update(obj); err != nil {
		slog.Warn("Failed to recompute object metadata", logutil.LogKeyTaskID, taskID, logutil.LogKeyURL, obj.URL, logutil.LogKeyError, err)
		return false
	}
	return true
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
