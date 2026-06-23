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

func (svc *AggregationService) AggregateObjects(page, limit int64, search, sortBy, status string, types []string) (map[string]any, error) {
	type taskInfo struct {
		t     core.Task
		count int64
	}
	var matchingTasks []taskInfo
	var total int64
	for _, t := range svc.tasks() {
		if !typeMatchesTask(t, types) {
			continue
		}
		countQuery := &core.StorageQuery{
			Filter: core.StorageFilter{
				Search: search,
			},
		}
		if status != "" && status != "all" {
			countQuery.Filter.Statuses = []string{status}
		}
		cnt, err := svc.count(t, countQuery)
		if err != nil {
			return nil, err
		}
		if cnt > 0 {
			matchingTasks = append(matchingTasks, taskInfo{t: t, count: cnt})
			total += cnt
		}
	}

	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = total
	}

	var all []*model.DownloadObject
	if limit > 0 && limit < total && len(matchingTasks) > 1 {
		allocated := int64(0)
		for i, ti := range matchingTasks {
			share := max(int64(1), limit*ti.count/total)
			if i == len(matchingTasks)-1 {
				share = max(0, limit-allocated)
			}
			if share <= 0 {
				continue
			}
			perTaskLimit := share * 3
			dataQuery := &core.StorageQuery{
				Filter: core.StorageFilter{
					Search: search,
				},
				Sort:  sortRules(sortBy),
				Limit: perTaskLimit,
			}
			if status != "" && status != "all" {
				dataQuery.Filter.Statuses = []string{status}
			}
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
			all = []*model.DownloadObject{}
		} else {
			end := min(offset+limit, int64(len(all)))
			all = all[offset:end]
		}
	} else {
		for _, ti := range matchingTasks {
			query := &core.StorageQuery{
				Filter: core.StorageFilter{
					Search: search,
				},
			}
			if status != "" && status != "all" {
				query.Filter.Statuses = []string{status}
			}
			objs, err := svc.collect(ti.t, query, 200)
			if err != nil {
				return nil, err
			}
			all = append(all, objs...)
		}
		offset := (page - 1) * limit
		paged := storage.ApplyQueryToObjects(all, &core.StorageQuery{
			Sort:   sortRules(sortBy),
			Offset: offset,
			Limit:  limit,
		})
		all = paged
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
