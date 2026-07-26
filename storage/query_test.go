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

func TestMatchTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		extra map[string]any
		tags  []string
		mode  string
		want  bool
	}{
		{
			name:  "mode any matches one tag",
			extra: map[string]any{"tags": []string{"action", "comedy"}},
			tags:  []string{"comedy"},
			mode:  "any",
			want:  true,
		},
		{
			name:  "mode any no match",
			extra: map[string]any{"tags": []string{"action", "drama"}},
			tags:  []string{"comedy"},
			mode:  "any",
			want:  false,
		},
		{
			name:  "mode all all match",
			extra: map[string]any{"tags": []string{"action", "comedy", "drama"}},
			tags:  []string{"action", "drama"},
			mode:  "all",
			want:  true,
		},
		{
			name:  "mode all partial match",
			extra: map[string]any{"tags": []string{"action", "comedy"}},
			tags:  []string{"action", "drama"},
			mode:  "all",
			want:  false,
		},
		{
			name:  "extra nil",
			extra: nil,
			tags:  []string{"action"},
			mode:  "any",
			want:  true,
		},
		{
			name:  "tags empty",
			extra: map[string]any{"tags": []string{"action"}},
			tags:  nil,
			mode:  "any",
			want:  true,
		},
		{
			name:  "extra missing tags key",
			extra: map[string]any{"other": "value"},
			tags:  []string{"action"},
			mode:  "any",
			want:  false,
		},
		{
			name:  "tags as []string",
			extra: map[string]any{"tags": []string{"action", "comedy"}},
			tags:  []string{"action"},
			mode:  "any",
			want:  true,
		},
		{
			name:  "tags as []any",
			extra: map[string]any{"tags": []any{"action", "comedy"}},
			tags:  []string{"comedy"},
			mode:  "any",
			want:  true,
		},
		{
			name:  "case insensitive any",
			extra: map[string]any{"tags": []string{"Action", "Comedy"}},
			tags:  []string{"action"},
			mode:  "any",
			want:  true,
		},
		{
			name:  "case insensitive all",
			extra: map[string]any{"tags": []string{"Action", "CoMedY"}},
			tags:  []string{"action", "comedy"},
			mode:  "all",
			want:  true,
		},
		{
			name:  "mode any matches one of many",
			extra: map[string]any{"tags": []string{"a", "b", "c"}},
			tags:  []string{"x", "y", "b", "z"},
			mode:  "any",
			want:  true,
		},
		{
			name:  "mode any all miss",
			extra: map[string]any{"tags": []string{"a", "b"}},
			tags:  []string{"x", "y", "z"},
			mode:  "any",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchTags(tt.extra, tt.tags, tt.mode)
			if got != tt.want {
				t.Errorf("matchTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplySort_Random(t *testing.T) {
	t.Parallel()

	t.Run("empty list", func(t *testing.T) {
		objects := []*model.DownloadObject{}
		query := &core.StorageQuery{Sort: []core.StorageSort{{Field: "random"}}}
		// Should not panic
		applySort(objects, query)
	})

	t.Run("single element", func(t *testing.T) {
		objects := []*model.DownloadObject{{URL: "http://a.com"}}
		query := &core.StorageQuery{Sort: []core.StorageSort{{Field: "random"}}}
		// Should not panic
		applySort(objects, query)
		if len(objects) != 1 {
			t.Fatalf("expected 1 object, got %d", len(objects))
		}
		if objects[0].URL != "http://a.com" {
			t.Errorf("expected URL unchanged, got %s", objects[0].URL)
		}
	})

	t.Run("multiple elements preserves count and URLs", func(t *testing.T) {
		objects := []*model.DownloadObject{
			{URL: "http://a.com"},
			{URL: "http://b.com"},
			{URL: "http://c.com"},
			{URL: "http://d.com"},
			{URL: "http://e.com"},
		}
		originalURLs := make(map[string]bool, len(objects))
		for _, obj := range objects {
			originalURLs[obj.URL] = true
		}

		query := &core.StorageQuery{Sort: []core.StorageSort{{Field: "random"}}}
		applySort(objects, query)

		if len(objects) != 5 {
			t.Fatalf("expected 5 objects, got %d", len(objects))
		}
		gotURLs := make(map[string]bool, len(objects))
		for _, obj := range objects {
			gotURLs[obj.URL] = true
		}
		for url := range originalURLs {
			if !gotURLs[url] {
				t.Errorf("missing URL %s after shuffle", url)
			}
		}
		if len(gotURLs) != len(originalURLs) {
			t.Errorf("URL set mismatch: got %d, want %d", len(gotURLs), len(originalURLs))
		}
	})
}

func TestMatchesFilterFields_TagsAndExcludeIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		obj    *model.DownloadObject
		filter core.StorageFilter
		want   bool
	}{
		{
			name: "ExcludeIDs match excludes object",
			obj:  &model.DownloadObject{URL: "http://a.com", ID: 5},
			filter: core.StorageFilter{
				ExcludeIDs: []int64{5, 10},
			},
			want: false,
		},
		{
			name: "ExcludeIDs no match includes object",
			obj:  &model.DownloadObject{URL: "http://a.com", ID: 5},
			filter: core.StorageFilter{
				ExcludeIDs: []int64{10, 15},
			},
			want: true,
		},
		{
			name: "Tags mode any match includes",
			obj:  &model.DownloadObject{URL: "http://a.com", Extra: map[string]any{"tags": []string{"action", "comedy"}}},
			filter: core.StorageFilter{
				Tags:    []string{"comedy"},
				TagMode: "any",
			},
			want: true,
		},
		{
			name: "Tags mode any no match excludes",
			obj:  &model.DownloadObject{URL: "http://a.com", Extra: map[string]any{"tags": []string{"action", "drama"}}},
			filter: core.StorageFilter{
				Tags:    []string{"comedy"},
				TagMode: "any",
			},
			want: false,
		},
		{
			name: "Tags mode all match includes",
			obj:  &model.DownloadObject{URL: "http://a.com", Extra: map[string]any{"tags": []string{"action", "comedy", "drama"}}},
			filter: core.StorageFilter{
				Tags:    []string{"action", "drama"},
				TagMode: "all",
			},
			want: true,
		},
		{
			name: "Tags mode all no match excludes",
			obj:  &model.DownloadObject{URL: "http://a.com", Extra: map[string]any{"tags": []string{"action", "comedy"}}},
			filter: core.StorageFilter{
				Tags:    []string{"action", "drama"},
				TagMode: "all",
			},
			want: false,
		},
		{
			name: "ExcludeIDs and Tags combined both match excludes",
			obj:  &model.DownloadObject{URL: "http://a.com", ID: 5, Extra: map[string]any{"tags": []string{"action"}}},
			filter: core.StorageFilter{
				ExcludeIDs: []int64{5},
				Tags:       []string{"action"},
				TagMode:    "any",
			},
			want: false,
		},
		{
			name: "ExcludeIDs no match and Tags match includes",
			obj:  &model.DownloadObject{URL: "http://a.com", ID: 5, Extra: map[string]any{"tags": []string{"action"}}},
			filter: core.StorageFilter{
				ExcludeIDs: []int64{10},
				Tags:       []string{"action"},
				TagMode:    "any",
			},
			want: true,
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

func TestMongoSortField_RandomAndTagMatchDesc(t *testing.T) {
	t.Parallel()
	// 在 no_mongo 构建下随机跳过（mongoSortField 仅在 mongo_storage.go 中定义）
	// 该测试在 mongo_storage_test.go 中也有完整覆盖
	t.Skip("mongoSortField 测试在 mongo_storage_test.go 中覆盖")
}
