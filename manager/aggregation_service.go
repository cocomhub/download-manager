// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"strings"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/storage"
)

// AggregationService handles read-only object aggregation and grouping queries.
type AggregationService struct {
	tasks   func() []core.Task
	search  func(t core.Task, query *core.StorageQuery) ([]*model.DownloadObject, error)
	count   func(t core.Task, query *core.StorageQuery) (int64, error)
	collect func(t core.Task, query *core.StorageQuery, batchSize int64) ([]*model.DownloadObject, error)
}

func NewAggregationService(
	getTasks func() []core.Task,
	search func(core.Task, *core.StorageQuery) ([]*model.DownloadObject, error),
	count func(core.Task, *core.StorageQuery) (int64, error),
	collect func(core.Task, *core.StorageQuery, int64) ([]*model.DownloadObject, error),
) *AggregationService {
	return &AggregationService{
		tasks:   getTasks,
		search:  search,
		count:   count,
		collect: collect,
	}
}

// taskInfo holds a task and the count of objects matching the current filter.
type taskInfo struct {
	t     core.Task
	count int64
}

func (svc *AggregationService) AggregateObjects(page, limit int64, search, sortBy, status string, types []string, tags string, tagMode string, excludeIDs []int64) (map[string]any, error) {
	matchingTasks, total, err := svc.collectMatchingTasks(search, status, types, tags, tagMode, excludeIDs)
	if err != nil {
		return nil, err
	}

	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = total
	}

	var all []*model.DownloadObject
	if limit > 0 && limit < total && len(matchingTasks) > 1 {
		all, err = svc.proportionalAllocation(matchingTasks, page, limit, total, search, status, sortBy, tags, tagMode, excludeIDs)
	} else {
		all, err = svc.simpleCollect(matchingTasks, page, limit, search, status, sortBy, tags, tagMode, excludeIDs)
	}
	if err != nil {
		return nil, err
	}

	if all == nil {
		all = make([]*model.DownloadObject, 0)
	}
	// Ensure every object has task_type metadata for frontend plugin dispatch
	for _, o := range all {
		if o.GetMetaTaskType() == "" {
			for _, ti := range matchingTasks {
				if ti.t.ID() == o.TaskID {
					o.EnsureTaskType(ti.t.Type())
					break
				}
			}
		}
	}
	resp := map[string]any{
		"objects": all,
		"total":   total,
		"page":    page,
		"limit":   limit,
	}
	// 搜索匹配信息：search 非空时计算每个对象的匹配字段，供前端展示高亮。
	if search != "" {
		if matched := matchedFieldsByObjects(all, search); len(matched) > 0 {
			resp["matched_fields"] = matched
		}
	}
	return resp, nil
}

// matchedFieldsByObjects 返回 URL → 匹配字段列表 的映射。
// 仅读取（不写存储），供响应附带展示用。
func matchedFieldsByObjects(objs []*model.DownloadObject, search string) map[string][]string {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return nil
	}
	result := make(map[string][]string)
	for _, o := range objs {
		if o == nil {
			continue
		}
		if fields := matchedFields(o, search); len(fields) > 0 {
			result[o.URL] = fields
		}
	}
	return result
}

// matchedFields 返回对象与搜索词匹配的字段列表。
// 与 storage.matchesQuery 的搜索语义保持一致（多字段 + tags）。
func matchedFields(o *model.DownloadObject, search string) []string {
	if o == nil {
		return nil
	}
	o.RLock()
	defer o.RUnlock()

	var fields []string
	if strings.Contains(strings.ToLower(o.URL), search) {
		fields = append(fields, "url")
	}
	if o.Metadata != nil {
		if strings.Contains(strings.ToLower(o.Metadata[model.MetadataKeyTitle]), search) {
			fields = append(fields, "title")
		}
		if strings.Contains(strings.ToLower(o.Metadata[model.MetadataKeyContentGroup]), search) {
			fields = append(fields, "content_group")
		}
		if strings.Contains(strings.ToLower(o.Metadata[model.MetadataKeyType]), search) {
			fields = append(fields, "task_type")
		}
		if strings.Contains(strings.ToLower(o.Metadata["date"]), search) {
			fields = append(fields, "date")
		}
	}
	for _, key := range []string{"page_url", "preview_url", "local_preview", "content_text", "content_html"} {
		if s, ok := o.Extra[key].(string); ok && strings.Contains(strings.ToLower(s), search) {
			fields = append(fields, key)
		}
	}
	if len(fields) == 0 {
		// 兜底：与 storage 搜索一致的 tags 匹配
		if tagsMatchSearch(o.Extra, search) {
			fields = append(fields, "tags")
		}
	}
	return fields
}

// tagsMatchSearch 检查 Extra.tags 是否包含搜索词（与 storage.extraTagsContain 一致）。
func tagsMatchSearch(extra map[string]any, search string) bool {
	raw, ok := extra["tags"]
	if !ok {
		return false
	}
	switch tags := raw.(type) {
	case []string:
		for _, tag := range tags {
			if strings.Contains(strings.ToLower(tag), search) {
				return true
			}
		}
	case []any:
		for _, tag := range tags {
			if tagStr, ok := tag.(string); ok && strings.Contains(strings.ToLower(tagStr), search) {
				return true
			}
		}
	case string:
		return strings.Contains(strings.ToLower(tags), search)
	}
	return false
}

// collectMatchingTasks filters tasks by type and counts their matching objects.
func (svc *AggregationService) collectMatchingTasks(search, status string, types []string, tags string, tagMode string, excludeIDs []int64) ([]taskInfo, int64, error) {
	var matchingTasks []taskInfo
	var total int64
	for _, t := range svc.tasks() {
		if !typeMatchesTask(t, types) {
			continue
		}
		cnt, err := svc.count(t, buildBaseQuery(search, status, tags, tagMode, excludeIDs))
		if err != nil {
			return nil, 0, err
		}
		if cnt > 0 {
			matchingTasks = append(matchingTasks, taskInfo{t: t, count: cnt})
			total += cnt
		}
	}
	return matchingTasks, total, nil
}

// proportionalAllocation 按各任务计数比例分配页内配额，并对每个任务用
// Search(Limit=share, Offset=(page-1)*share) 定向取数（取代旧实现的
// limit*3 超集拉取 + 内存切片），合并后全局排序得到当前页。
//
// 注意：配额窗口是「按源比例」的近似分页——当对象在各任务的分布与其
// 计数比例差异悬殊时，页边界可能与全局精确排序略有偏移（旧实现用 3x
// 超集缓解同一近似；新实现把每任务取数限制到页配额内，内存/传输显著下降）。
func (svc *AggregationService) proportionalAllocation(matchingTasks []taskInfo, page, limit, total int64, search, status, sortBy string, tags string, tagMode string, excludeIDs []int64) ([]*model.DownloadObject, error) {
	offset := (page - 1) * limit
	if offset >= total {
		return []*model.DownloadObject{}, nil
	}
	var all []*model.DownloadObject
	allocated := int64(0)
	for i, ti := range matchingTasks {
		share := max(int64(1), limit*ti.count/total)
		if i == len(matchingTasks)-1 {
			share = max(0, limit-allocated)
		}
		if share <= 0 {
			continue
		}
		// Use the tags/tagMode/excludeIDs from the original query by passing empty strings
		// since they are already baked into the collectMatchingTasks count call.
		dataQuery := buildBaseQuery(search, status, tags, tagMode, excludeIDs)
		dataQuery.Sort = sortRules(sortBy)
		dataQuery.Limit = share
		dataQuery.Offset = offset / limit * share
		objs, err := svc.search(ti.t, dataQuery)
		if err != nil {
			return nil, err
		}
		all = append(all, objs...)
		allocated += share
	}
	if len(all) == 0 {
		return []*model.DownloadObject{}, nil
	}
	if len(all) > 1 {
		all = storage.ApplyQueryToObjects(all, &core.StorageQuery{Sort: sortRules(sortBy)})
	}
	// 兜底截断：任务数 > limit+1 时前置任务的 share 被 max(1,...) 兜底到 1，
	// 合并结果可能超过 limit（page 已全局排序，截断即得精确页）。
	if int64(len(all)) > limit {
		all = all[:limit]
	}
	return all, nil
}

// simpleCollect gathers objects from every matching task,
// then sorts and paginates the merged result in a single pass.
// 单任务场景（无需跨任务合并排序）：直接下推 limit/offset 到后端 Search，
// 避免全量收集后再内存分页（大数据量下显著降低传输与排序成本）。
func (svc *AggregationService) simpleCollect(matchingTasks []taskInfo, page, limit int64, search, status, sortBy string, tags string, tagMode string, excludeIDs []int64) ([]*model.DownloadObject, error) {
	// 单任务 + 有排序/分页需求 → 后端下推（limit/offset/sort 由 storage.Search 执行）
	if len(matchingTasks) == 1 && limit > 0 {
		q := buildBaseQuery(search, status, tags, tagMode, excludeIDs)
		q.Sort = sortRules(sortBy)
		q.Limit = limit
		q.Offset = (page - 1) * limit
		objs, err := svc.search(matchingTasks[0].t, q)
		if err != nil {
			return nil, err
		}
		if objs == nil {
			objs = []*model.DownloadObject{}
		}
		return objs, nil
	}

	var all []*model.DownloadObject
	for _, ti := range matchingTasks {
		objs, err := svc.collect(ti.t, buildBaseQuery(search, status, tags, tagMode, excludeIDs), 200)
		if err != nil {
			return nil, err
		}
		all = append(all, objs...)
	}
	offset := (page - 1) * limit
	return storage.ApplyQueryToObjects(all, &core.StorageQuery{
		Sort:   sortRules(sortBy),
		Offset: offset,
		Limit:  limit,
	}), nil
}

// buildBaseQuery creates a StorageQuery with search filter, status filter, tags, tag mode, and exclude IDs.
func buildBaseQuery(search, status string, tags string, tagMode string, excludeIDs []int64) *core.StorageQuery {
	q := &core.StorageQuery{
		Filter: core.StorageFilter{
			Search:     search,
			Tags:       parseTags(tags),
			TagMode:    tagMode,
			ExcludeIDs: excludeIDs,
		},
	}
	if status != "" && status != "all" {
		q.Filter.Statuses = []string{status}
	}
	return q
}

// parseTags splits a comma-separated tags string into a slice.
func parseTags(tags string) []string {
	if tags == "" {
		return nil
	}
	var result []string
	for t := range strings.SplitSeq(tags, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}
