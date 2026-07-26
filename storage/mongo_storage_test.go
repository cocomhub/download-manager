// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build !no_mongo

package storage

import (
	"testing"

	"github.com/cocomhub/download-manager/core"
	"go.mongodb.org/mongo-driver/bson"
)

func TestNormalizeMongoQuery_DefaultLimitAndSort(t *testing.T) {
	got := normalizeMongoQuery(nil)
	if got.Limit != 200 {
		t.Fatalf("expected default limit 200, got %d", got.Limit)
	}
	if len(got.Sort) != 2 || got.Sort[0].Field != "date" || !got.Sort[0].Desc {
		t.Fatalf("expected default date desc sort, got %+v", got.Sort)
	}
}

func TestNormalizeMongoQuery_ClampLargeLimit(t *testing.T) {
	got := normalizeMongoQuery(&core.StorageQuery{Limit: 4096})
	if got.Limit != 1000 {
		t.Fatalf("expected clamped limit 1000, got %d", got.Limit)
	}
}

func TestBuildMongoFilter_IncludesTaskStatusMetadataAndSearch(t *testing.T) {
	filter := buildMongoFilter(&core.StorageQuery{
		Filter: core.StorageFilter{
			TaskIDs:  []string{"t1"},
			Statuses: []string{"pending"},
			Metadata: map[string]string{"content_group": "ABP-123"},
			Search:   "club",
		},
	})
	if filter["task_id"] == nil {
		t.Fatalf("expected task_id filter")
	}
	if filter["status"] == nil {
		t.Fatalf("expected status filter")
	}
	if got := filter["metadata.content_group"]; got != "ABP-123" {
		t.Fatalf("expected metadata filter, got %v", got)
	}
	andVal, ok := filter["$and"].(bson.A)
	if !ok || len(andVal) != 1 {
		t.Fatalf("expected $and array with 1 condition, got %T %+v", filter["$and"], filter["$and"])
	}
	searchOr, ok := andVal[0].(bson.M)
	if !ok {
		t.Fatalf("expected bson.M in $and[0], got %T", andVal[0])
	}
	orVal, ok := searchOr["$or"].(bson.A)
	if !ok || len(orVal) != 3 {
		t.Fatalf("expected 3-way search OR inside $and, got %T %+v", searchOr["$or"], searchOr["$or"])
	}
}

func TestBuildMongoFilter_MissingID(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name  string
		query *core.StorageQuery
		check func(t *testing.T, filter bson.M)
	}{
		{
			name: "missingID true adds $and with $or for missing id",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{MissingID: &trueVal},
			},
			check: func(t *testing.T, filter bson.M) {
				andVal, ok := filter["$and"].(bson.A)
				if !ok {
					t.Fatalf("expected $and array, got %T", filter["$and"])
				}
				if len(andVal) != 1 {
					t.Fatalf("expected 1 condition in $and, got %d", len(andVal))
				}
				// First condition should be $or with id missing/id=0
				orCond, ok := andVal[0].(bson.M)
				if !ok {
					t.Fatalf("expected bson.M in $and[0], got %T", andVal[0])
				}
				orVal, ok := orCond["$or"].(bson.A)
				if !ok {
					t.Fatalf("expected $or array inside $and, got %T", orCond["$or"])
				}
				if len(orVal) != 2 {
					t.Fatalf("expected 2 conditions in $or, got %d", len(orVal))
				}
				foundExists := false
				foundZero := false
				for _, cond := range orVal {
					condMap, ok := cond.(bson.M)
					if !ok {
						continue
					}
					if existsVal, ok := condMap["id"].(bson.M); ok {
						if existsVal["$exists"] == false {
							foundExists = true
						}
					}
					if condMap["id"] == 0 {
						foundZero = true
					}
				}
				if !foundExists {
					t.Error("expected $or condition: id {$exists: false}")
				}
				if !foundZero {
					t.Error("expected $or condition: id = 0")
				}
			},
		},
		{
			name: "missingID false adds $and with id exists and != 0",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{MissingID: &falseVal},
			},
			check: func(t *testing.T, filter bson.M) {
				andVal, ok := filter["$and"].(bson.A)
				if !ok {
					t.Fatalf("expected $and array, got %T", filter["$and"])
				}
				if len(andVal) != 1 {
					t.Fatalf("expected 1 condition in $and, got %d", len(andVal))
				}
				idCond, ok := andVal[0].(bson.M)
				if !ok {
					t.Fatalf("expected bson.M in $and[0], got %T", andVal[0])
				}
				idVal, ok := idCond["id"].(bson.M)
				if !ok {
					t.Fatalf("expected id field condition, got %T", idCond["id"])
				}
				if idVal["$exists"] != true {
					t.Errorf("expected $exists: true, got %v", idVal["$exists"])
				}
			},
		},
		{
			name: "missingID true with search combines via $and",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					MissingID: &trueVal,
					Search:    "test",
				},
			},
			check: func(t *testing.T, filter bson.M) {
				andVal, ok := filter["$and"].(bson.A)
				if !ok {
					t.Fatalf("expected $and array, got %T", filter["$and"])
				}
				// Should have 2 conditions: 1 for MissingID $or, 1 for Search $or
				if len(andVal) != 2 {
					t.Fatalf("expected 2 conditions in $and, got %d", len(andVal))
				}
			},
		},
		{
			name: "Tags mode any constructs $or inside $and",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					Tags:    []string{"action", "comedy"},
					TagMode: "any",
				},
			},
			check: func(t *testing.T, filter bson.M) {
				andVal, ok := filter["$and"].(bson.A)
				if !ok || len(andVal) != 1 {
					t.Fatalf("expected $and array with 1 condition, got %T", filter["$and"])
				}
				tagOr, ok := andVal[0].(bson.M)
				if !ok {
					t.Fatalf("expected bson.M in $and[0], got %T", andVal[0])
				}
				orVal, ok := tagOr["$or"].(bson.A)
				if !ok || len(orVal) != 2 {
					t.Fatalf("expected $or with 2 conditions, got %T %+v", tagOr["$or"], tagOr["$or"])
				}
			},
		},
		{
			name: "Tags mode all constructs per-tag conditions inside $and",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					Tags:    []string{"action", "comedy"},
					TagMode: "all",
				},
			},
			check: func(t *testing.T, filter bson.M) {
				andVal, ok := filter["$and"].(bson.A)
				if !ok || len(andVal) != 2 {
					t.Fatalf("expected $and array with 2 tag conditions, got %d", len(andVal))
				}
				for i, cond := range andVal {
					condMap, ok := cond.(bson.M)
					if !ok {
						t.Fatalf("expected bson.M in $and[%d], got %T", i, cond)
					}
					tagCond, ok := condMap["extra.tags"].(bson.M)
					if !ok {
						t.Fatalf("expected extra.tags regex in $and[%d], got %T", i, condMap["extra.tags"])
					}
					if tagCond["$options"] != "i" {
						t.Errorf("expected $options 'i', got %v", tagCond["$options"])
					}
				}
			},
		},
		{
			name: "ExcludeIDs constructs $nin filter",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					ExcludeIDs: []int64{1, 2, 3},
				},
			},
			check: func(t *testing.T, filter bson.M) {
				ninVal, ok := filter["id"].(bson.M)
				if !ok {
					t.Fatalf("expected id filter, got %T", filter["id"])
				}
				nin, ok := ninVal["$nin"].([]int64)
				if !ok || len(nin) != 3 {
					t.Fatalf("expected $nin with 3 values, got %T %+v", ninVal["$nin"], ninVal["$nin"])
				}
			},
		},
		{
			name: "Tags and ExcludeIDs combined",
			query: &core.StorageQuery{
				Filter: core.StorageFilter{
					Tags:       []string{"action"},
					TagMode:    "any",
					ExcludeIDs: []int64{5},
				},
			},
			check: func(t *testing.T, filter bson.M) {
				// ExcludeIDs should be top-level
				ninVal, ok := filter["id"].(bson.M)
				if !ok {
					t.Fatalf("expected id filter, got %T", filter["id"])
				}
				if _, ok := ninVal["$nin"]; !ok {
					t.Fatalf("expected $nin in id filter")
				}
				// Tags should be in $and
				andVal, ok := filter["$and"].(bson.A)
				if !ok || len(andVal) != 1 {
					t.Fatalf("expected $and with 1 condition for tags, got %d", len(andVal))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := buildMongoFilter(tt.query)
			tt.check(t, filter)
		})
	}
}

func TestBuildMongoSort_MapsKnownFields(t *testing.T) {
	got := buildMongoSort([]core.StorageSort{
		{Field: "date", Desc: true},
		{Field: "url"},
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 sort fields, got %d", len(got))
	}
	if got[0].Key != "metadata.date" || got[0].Value != -1 {
		t.Fatalf("unexpected first sort field: %+v", got[0])
	}
	if got[1].Key != "url" || got[1].Value != 1 {
		t.Fatalf("unexpected second sort field: %+v", got[1])
	}
}

func TestNormalizeMongoQuery_DeepCopyNewFields(t *testing.T) {
	// Verify that Tags, TagMode, and ExcludeIDs are deep-copied correctly
	orig := &core.StorageQuery{
		Filter: core.StorageFilter{
			Tags:       []string{"action", "comedy"},
			TagMode:    "any",
			ExcludeIDs: []int64{1, 2, 3},
		},
	}
	cloned := normalizeMongoQuery(orig)
	// Modify original slices
	orig.Filter.Tags[0] = "modified"
	orig.Filter.ExcludeIDs[0] = 999
	// Cloned should be unaffected
	if cloned.Filter.Tags[0] != "action" {
		t.Errorf("Tags deep copy failed: got %q, want 'action'", cloned.Filter.Tags[0])
	}
	if cloned.Filter.ExcludeIDs[0] != 1 {
		t.Errorf("ExcludeIDs deep copy failed: got %d, want 1", cloned.Filter.ExcludeIDs[0])
	}
	if cloned.Filter.TagMode != "any" {
		t.Errorf("TagMode copied: got %q, want 'any'", cloned.Filter.TagMode)
	}
}
