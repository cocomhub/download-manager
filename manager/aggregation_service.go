// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
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

func (svc *AggregationService) AggregateObjects(page, limit int64, search, sortBy, status string, types []string) (map[string]any, error) {
	matchingTasks, total, err := svc.collectMatchingTasks(search, status, types)
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
		all, err = svc.proportionalAllocation(matchingTasks, page, limit, total, search, status, sortBy)
	} else {
		all, err = svc.simpleCollect(matchingTasks, page, limit, search, status, sortBy)
	}
	if err != nil {
		return nil, err
	}

	if all == nil {
		all = make([]*model.DownloadObject, 0)
	}
	return map[string]any{
		"objects": all,
		"total":   total,
		"page":    page,
		"limit":   limit,
	}, nil
}

// collectMatchingTasks filters tasks by type and counts their matching objects.
func (svc *AggregationService) collectMatchingTasks(search, status string, types []string) ([]taskInfo, int64, error) {
	var matchingTasks []taskInfo
	var total int64
	for _, t := range svc.tasks() {
		if !typeMatchesTask(t, types) {
			continue
		}
		cnt, err := svc.count(t, buildBaseQuery(search, status))
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

// proportionalAllocation fetches a superset of objects from each task proportionally,
// then sorts and paginates the merged result.
func (svc *AggregationService) proportionalAllocation(matchingTasks []taskInfo, page, limit, total int64, search, status, sortBy string) ([]*model.DownloadObject, error) {
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
		dataQuery := buildBaseQuery(search, status)
		dataQuery.Sort = sortRules(sortBy)
		dataQuery.Limit = share * 3
		objs, err := svc.search(ti.t, dataQuery)
		if err != nil {
			return nil, err
		}
		all = append(all, objs...)
		allocated += share
	}
	if len(all) > 1 {
		storage.ApplyQueryToObjects(all, &core.StorageQuery{Sort: sortRules(sortBy)})
	}
	offset := (page - 1) * limit
	if offset >= int64(len(all)) {
		return []*model.DownloadObject{}, nil
	}
	end := min(offset+limit, int64(len(all)))
	return all[offset:end], nil
}

// simpleCollect gathers all objects from every matching task,
// then sorts and paginates the merged result in a single pass.
func (svc *AggregationService) simpleCollect(matchingTasks []taskInfo, page, limit int64, search, status, sortBy string) ([]*model.DownloadObject, error) {
	var all []*model.DownloadObject
	for _, ti := range matchingTasks {
		objs, err := svc.collect(ti.t, buildBaseQuery(search, status), 200)
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

// buildBaseQuery creates a StorageQuery with search filter and optional status filter.
func buildBaseQuery(search, status string) *core.StorageQuery {
	q := &core.StorageQuery{
		Filter: core.StorageFilter{
			Search: search,
		},
	}
	if status != "" && status != "all" {
		q.Filter.Statuses = []string{status}
	}
	return q
}
