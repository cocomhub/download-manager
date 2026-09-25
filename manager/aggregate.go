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
	"github.com/cocomhub/download-manager/storage"
)

func (m *Manager) AggregateObjects(page, limit int64, search, sortBy, status string, types []string, tags string, tagMode string, excludeIDs []int64) (map[string]any, error) {
	return m.aggSvc.AggregateObjects(page, limit, search, sortBy, status, types, tags, tagMode, excludeIDs)
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
		Light:  true, // 剔除 extra.files/images/links 大数组，减小传输/解码开销
	}
	if status != "" && status != "all" {
		q.Filter.Statuses = []string{status}
	}
	return q
}

// collectTaskGroupEntries 单任务内存分组（快路径失败/自定义语义任务用）：
// 返回该任务的 content_group 分组结果，供 selectGroupRepresentatives 选代表。
func collectTaskGroupEntries(m *Manager, tk core.Task, search, status string) map[string][]contentGroupEntry {
	objs, err := m.collectTaskObjects(tk, buildContentQuery(search, status), 200)
	if err != nil {
		return nil
	}
	entries := make([]contentGroupEntry, 0, len(objs))
	for _, o := range objs {
		entries = append(entries, contentGroupEntry{task: tk, obj: o})
	}
	return groupByContentKey(entries)
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
	bestDate := ""
	for idx, e := range entries {
		// 有自定义代表语义（ContentGroupProvider.VariantScore）→ 用分数选代表；
		// 否则（idx==0 兜底 + 无分数）选 metadata.date 最大者，与书橱视图/存储层快路径一致。
		if cgp, ok := e.task.(core.ContentGroupProvider); ok && cgp.VariantScore(e.obj) > 0 {
			score := cgp.VariantScore(e.obj)
			if idx == 0 || score > bestScore {
				rep = e.obj
				bestScore = score
			}
			continue
		}
		date := metadataDate(e.obj)
		if idx == 0 || date > bestDate {
			rep = e.obj
			bestDate = date
		}
	}
	return rep
}

// metadataDate 返回对象的 metadata.date（RLock 保护）。
func metadataDate(obj *model.DownloadObject) string {
	if obj == nil {
		return ""
	}
	obj.RLock()
	defer obj.RUnlock()
	if obj.Metadata == nil {
		return ""
	}
	return obj.Metadata["date"]
}

// copyRepresentative creates a shallow copy of rep and attaches the group_size extra field.
func copyRepresentative(rep *model.DownloadObject, groupSize int) *model.DownloadObject {
	c := &model.DownloadObject{
		TaskID:   rep.TaskID,
		URL:      rep.URL,
		SavePath: rep.SavePath,
		Status:   rep.GetStatus(),
		Progress: rep.GetProgress(),
		Version:  rep.GetVersion(),
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

	// 快路径：每个匹配任务若存储实现 ContentGroupRepresentatives 且任务无自定义代表语义
	// （未实现 ContentGroupProvider）→ mongo 聚合下推，避免全量拉取内存分组。
	// 多任务时逐任务独立决策：能下推的先下推（每任务只取代表，不分页），
	// 自定义/不支持的走内存（collectTaskGroupReps），最后合并统一分页。
	var reps []*model.DownloadObject
	var repGroups []map[string][]contentGroupEntry
	for _, tk := range matchingTasks {
		_, hasCustom := tk.(core.ContentGroupProvider)
		if st, ok := tk.Storage().(core.ContentGroupRepresentatives); ok && !hasCustom {
			objs, _, err := st.ContentGroupRepresentatives(tk.ID(), search, status, 1, core.NoLimit)
			if err != nil {
				// 快路径失败 → 该任务回退内存
				repGroups = append(repGroups, collectTaskGroupEntries(m, tk, search, status))
				continue
			}
			for _, o := range objs {
				if o.GetMetaTaskType() == "" {
					o.EnsureTaskType(tk.Type())
				}
			}
			reps = append(reps, objs...)
			continue
		}
		// 自定义代表语义或存储不支持 → 内存分组（保留该任务的组内代表语义）
		repGroups = append(repGroups, collectTaskGroupEntries(m, tk, search, status))
	}
	if len(repGroups) > 0 {
		// 有任务走内存：合并内存组（含快路径已取代表），统一选代表/分页
		for _, g := range repGroups {
			reps = append(reps, selectGroupRepresentatives(g)...)
		}
		// 合并后的代表统一分页
		paged, total, outPage, outLimit := paginateContentResults(reps, page, limit, sortBy)
		ensurePagedTaskTypes(paged, matchingTasks)
		return map[string]any{
			"objects": paged,
			"total":   total,
			"page":    outPage,
			"limit":   outLimit,
		}, nil
	}

	// 全部任务走快路径 → 已取每任务代表，内存统一分页
	paged, total, outPage, outLimit := paginateContentResults(reps, page, limit, sortBy)
	return map[string]any{
		"objects": paged,
		"total":   total,
		"page":    outPage,
		"limit":   outLimit,
	}, nil
}

// ensurePagedTaskTypes 确保分页结果每个对象带 task_type 元数据（前端插件分发用）。
func ensurePagedTaskTypes(paged []*model.DownloadObject, matchingTasks []core.Task) {
	for _, o := range paged {
		if o.GetMetaTaskType() == "" {
			for _, tk := range matchingTasks {
				if tk.ID() == o.TaskID {
					o.EnsureTaskType(tk.Type())
					break
				}
			}
		}
	}
}

func metadataContentGroup(obj *model.DownloadObject) string {
	if obj == nil {
		return ""
	}
	obj.RLock()
	defer obj.RUnlock()
	if obj.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(obj.Metadata[model.MetadataKeyContentGroup])
}

func metadataTaskType(obj *model.DownloadObject) string {
	if obj == nil {
		return ""
	}
	obj.RLock()
	defer obj.RUnlock()
	if obj.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(obj.Metadata["task_type"])
}

func scopedContentGroupKey(taskID, taskType, group string) string {
	return strings.TrimSpace(taskID) + "\x00" + strings.TrimSpace(taskType) + "\x00" + strings.TrimSpace(group)
}

// variantPriorityScore 返回对象在内容组内的变体优先级。
// 任务实现 core.ContentGroupProvider 时按接口逻辑评分（如 tktube 高画质/中字）；
// 未实现的任务返回 0（默认不参与变体淘汰）。
func variantPriorityScore(t core.Task, obj *model.DownloadObject) int {
	if t == nil || obj == nil {
		return 0
	}
	if cgp, ok := t.(core.ContentGroupProvider); ok {
		return cgp.VariantScore(obj)
	}
	return 0
}

// BackfillContentGroups scans storages and recomputes content_group/task_type metadata for tktube tasks.
func (m *Manager) BackfillContentGroups() {
	m.tasks.Range(func(key, value any) bool {
		m.processOneBackfillTask(value)
		return true
	})
}

// processOneBackfillTask processes a single value from the tasks map during backfill.
// 仅处理实现 core.ContentGroupProvider 且 BackfillContentGroups()=true 的任务。
func (m *Manager) processOneBackfillTask(value any) bool {
	t, _ := value.(core.Task)
	if t == nil {
		return true
	}
	cgp, ok := t.(core.ContentGroupProvider)
	if !ok || !cgp.BackfillContentGroups() {
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
		if applyBackfillMetadata(cgp, obj, taskType, t.ID(), st) {
			changed++
		}
	}
	slog.Info("Recomputed object metadata", logutil.LogKeyTaskID, t.ID(), "task_type", t.Type(), "total", total, "changed", changed)
	return true
}

// applyBackfillMetadata computes content_group and task_type metadata via the task's
// ContentGroupProvider and persists it if changed. Returns true if a change was made.
func applyBackfillMetadata(cgp core.ContentGroupProvider, obj *model.DownloadObject, taskType, taskID string, st core.Storage) bool {
	obj.Lock()
	if obj.Metadata == nil {
		obj.Metadata = make(map[string]string)
	}
	newGroup := cgp.ContentGroupKey(obj)
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
