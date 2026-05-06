// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

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
	if got := filter["task_id"]; got == nil {
		t.Fatalf("expected task_id filter")
	}
	if got := filter["status"]; got == nil {
		t.Fatalf("expected status filter")
	}
	if got := filter["metadata.content_group"]; got != "ABP-123" {
		t.Fatalf("expected metadata filter, got %v", got)
	}
	orVal, ok := filter["$or"].(bson.A)
	if !ok || len(orVal) != 3 {
		t.Fatalf("expected 3-way search OR, got %T %+v", filter["$or"], filter["$or"])
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
