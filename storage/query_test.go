// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"testing"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

func TestMatchesFilterFields_MissingID(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name   string
		obj    *model.DownloadObject
		filter core.StorageFilter
		want   bool
	}{
		{
			name:   "missingID true with id=0 includes",
			obj:    &model.DownloadObject{URL: "http://a.com", ID: 0},
			filter: core.StorageFilter{MissingID: &trueVal},
			want:   true,
		},
		{
			name:   "missingID true with id=5 excludes",
			obj:    &model.DownloadObject{URL: "http://a.com", ID: 5},
			filter: core.StorageFilter{MissingID: &trueVal},
			want:   false,
		},
		{
			name:   "missingID false with id=0 excludes",
			obj:    &model.DownloadObject{URL: "http://a.com", ID: 0},
			filter: core.StorageFilter{MissingID: &falseVal},
			want:   false,
		},
		{
			name:   "missingID false with id=5 includes",
			obj:    &model.DownloadObject{URL: "http://a.com", ID: 5},
			filter: core.StorageFilter{MissingID: &falseVal},
			want:   true,
		},
		{
			name:   "missingID nil with id=0 includes",
			obj:    &model.DownloadObject{URL: "http://a.com", ID: 0},
			filter: core.StorageFilter{},
			want:   true,
		},
		{
			name:   "missingID nil with id=5 includes",
			obj:    &model.DownloadObject{URL: "http://a.com", ID: 5},
			filter: core.StorageFilter{},
			want:   true,
		},
		{
			name:   "missingID true with status filter still works",
			obj:    &model.DownloadObject{URL: "http://a.com", ID: 0, Status: model.StatusPending},
			filter: core.StorageFilter{MissingID: &trueVal, Statuses: []string{model.StatusCompleted}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesFilterFields(tt.obj, tt.filter)
			if got != tt.want {
				t.Errorf("matchesFilterFields() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyQueryToObjects_MissingID(t *testing.T) {
	trueVal := true
	falseVal := false

	objects := []*model.DownloadObject{
		{URL: "http://a.com", ID: 0, TaskID: "t1"},
		{URL: "http://b.com", ID: 5, TaskID: "t1"},
		{URL: "http://c.com", ID: 0, TaskID: "t2"},
		{URL: "http://d.com", ID: 10, TaskID: "t2"},
	}

	tests := []struct {
		name  string
		query *core.StorageQuery
		want  int
	}{
		{
			name:  "missingID true returns only objects with id=0",
			query: &core.StorageQuery{Filter: core.StorageFilter{MissingID: &trueVal}},
			want:  2,
		},
		{
			name:  "missingID false returns only objects with id!=0",
			query: &core.StorageQuery{Filter: core.StorageFilter{MissingID: &falseVal}},
			want:  2,
		},
		{
			name:  "missingID true with task filter",
			query: &core.StorageQuery{Filter: core.StorageFilter{MissingID: &trueVal, TaskIDs: []string{"t1"}}},
			want:  1,
		},
		{
			name:  "missingID nil returns all",
			query: &core.StorageQuery{Filter: core.StorageFilter{}},
			want:  4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyQueryToObjects(objects, tt.query)
			if len(got) != tt.want {
				t.Errorf("ApplyQueryToObjects() returned %d objects, want %d", len(got), tt.want)
			}
		})
	}
}

func TestMongoSortField_RandomAndTagMatchDesc(t *testing.T) {
	// random and tag_match_desc should return empty string (handled in memory)
	if got := mongoSortField("random"); got != "" {
		t.Errorf("mongoSortField('random') = %q, want ''", got)
	}
	if got := mongoSortField("tag_match_desc"); got != "" {
		t.Errorf("mongoSortField('tag_match_desc') = %q, want ''", got)
	}
}
