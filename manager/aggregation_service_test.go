// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/core"
)

// =============================================================================
// parseTags
// =============================================================================

func TestParseTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags string
		want []string
	}{
		{
			name: "empty string returns nil",
			tags: "",
			want: nil,
		},
		{
			name: "single tag",
			tags: "tag1",
			want: []string{"tag1"},
		},
		{
			name: "multiple tags",
			tags: "tag1,tag2,tag3",
			want: []string{"tag1", "tag2", "tag3"},
		},
		{
			name: "skips empty parts",
			tags: "tag1,,tag2",
			want: []string{"tag1", "tag2"},
		},
		{
			name: "trims whitespace",
			tags: " tag1 , tag2 ",
			want: []string{"tag1", "tag2"},
		},
		{
			name: "all empty parts returns nil",
			tags: ",,,",
			want: nil,
		},
		{
			name: "single tag with spaces",
			tags: "  hello world  ",
			want: []string{"hello world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTags(tt.tags)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseTags(%q) = %v (len=%d), want %v (len=%d)", tt.tags, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseTags(%q)[%d] = %q, want %q", tt.tags, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// =============================================================================
// buildBaseQuery
// =============================================================================

func TestBuildBaseQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		search     string
		status     string
		tags       string
		tagMode    string
		excludeIDs []int64
		want       *core.StorageQuery
	}{
		{
			name:   "search only",
			search: "keyword",
			status: "",
			tags:   "",
			want: &core.StorageQuery{
				Filter: core.StorageFilter{
					Search:     "keyword",
					Tags:       nil,
					TagMode:    "",
					ExcludeIDs: nil,
				},
			},
		},
		{
			name:   "search with status",
			search: "test",
			status: "completed",
			tags:   "",
			want: &core.StorageQuery{
				Filter: core.StorageFilter{
					Search:     "test",
					Statuses:   []string{"completed"},
					Tags:       nil,
					TagMode:    "",
					ExcludeIDs: nil,
				},
			},
		},
		{
			name:   "status all clears status filter",
			search: "",
			status: "all",
			tags:   "",
			want: &core.StorageQuery{
				Filter: core.StorageFilter{
					Search:     "",
					Tags:       nil,
					TagMode:    "",
					ExcludeIDs: nil,
				},
			},
		},
		{
			name:    "tags with tagMode",
			search:  "",
			status:  "",
			tags:    "tag1,tag2",
			tagMode: "any",
			want: &core.StorageQuery{
				Filter: core.StorageFilter{
					Search:     "",
					Tags:       []string{"tag1", "tag2"},
					TagMode:    "any",
					ExcludeIDs: nil,
				},
			},
		},
		{
			name:       "excludeIDs",
			search:     "",
			status:     "",
			tags:       "",
			excludeIDs: []int64{1, 2, 3},
			want: &core.StorageQuery{
				Filter: core.StorageFilter{
					Search:     "",
					Tags:       nil,
					TagMode:    "",
					ExcludeIDs: []int64{1, 2, 3},
				},
			},
		},
		{
			name:       "all parameters combined",
			search:     "query",
			status:     "pending",
			tags:       "action,comedy",
			tagMode:    "all",
			excludeIDs: []int64{99},
			want: &core.StorageQuery{
				Filter: core.StorageFilter{
					Search:     "query",
					Statuses:   []string{"pending"},
					Tags:       []string{"action", "comedy"},
					TagMode:    "all",
					ExcludeIDs: []int64{99},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildBaseQuery(tt.search, tt.status, tt.tags, tt.tagMode, tt.excludeIDs)
			if got.Filter.Search != tt.want.Filter.Search {
				t.Errorf("Search = %q, want %q", got.Filter.Search, tt.want.Filter.Search)
			}
			if len(got.Filter.Statuses) != len(tt.want.Filter.Statuses) {
				t.Errorf("Statuses = %v, want %v", got.Filter.Statuses, tt.want.Filter.Statuses)
			} else {
				for i := range got.Filter.Statuses {
					if got.Filter.Statuses[i] != tt.want.Filter.Statuses[i] {
						t.Errorf("Statuses[%d] = %q, want %q", i, got.Filter.Statuses[i], tt.want.Filter.Statuses[i])
					}
				}
			}
			if len(got.Filter.Tags) != len(tt.want.Filter.Tags) {
				t.Errorf("Tags = %v, want %v", got.Filter.Tags, tt.want.Filter.Tags)
			} else {
				for i := range got.Filter.Tags {
					if got.Filter.Tags[i] != tt.want.Filter.Tags[i] {
						t.Errorf("Tags[%d] = %q, want %q", i, got.Filter.Tags[i], tt.want.Filter.Tags[i])
					}
				}
			}
			if got.Filter.TagMode != tt.want.Filter.TagMode {
				t.Errorf("TagMode = %q, want %q", got.Filter.TagMode, tt.want.Filter.TagMode)
			}
			if len(got.Filter.ExcludeIDs) != len(tt.want.Filter.ExcludeIDs) {
				t.Errorf("ExcludeIDs = %v, want %v", got.Filter.ExcludeIDs, tt.want.Filter.ExcludeIDs)
			} else {
				for i := range got.Filter.ExcludeIDs {
					if got.Filter.ExcludeIDs[i] != tt.want.Filter.ExcludeIDs[i] {
						t.Errorf("ExcludeIDs[%d] = %d, want %d", i, got.Filter.ExcludeIDs[i], tt.want.Filter.ExcludeIDs[i])
					}
				}
			}
		})
	}
}
