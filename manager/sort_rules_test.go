// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"testing"

	"github.com/cocomhub/download-manager/core"
)

// =============================================================================
// sortRules
// =============================================================================

func TestSortRules_Random(t *testing.T) {
	t.Parallel()

	got := sortRules("random")
	want := []core.StorageSort{{Field: "random"}}
	if len(got) != len(want) {
		t.Fatalf("sortRules(\"random\") = %v (len=%d), want %v (len=%d)", got, len(got), want, len(want))
	}
	for i := range got {
		if got[i].Field != want[i].Field || got[i].Desc != want[i].Desc {
			t.Errorf("sortRules(\"random\")[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSortRules_TagMatchDesc(t *testing.T) {
	t.Parallel()

	got := sortRules("tag_match_desc")
	want := []core.StorageSort{
		{Field: "tag_match_desc", Desc: true},
		{Field: "date", Desc: true},
		{Field: "url"},
	}
	if len(got) != len(want) {
		t.Fatalf("sortRules(\"tag_match_desc\") = %v (len=%d), want %v (len=%d)", got, len(got), want, len(want))
	}
	for i := range got {
		if got[i].Field != want[i].Field || got[i].Desc != want[i].Desc {
			t.Errorf("sortRules(\"tag_match_desc\")[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSortRules_EmptyDefaultsToDateDesc(t *testing.T) {
	t.Parallel()

	got := sortRules("")
	want := []core.StorageSort{{Field: "date", Desc: true}, {Field: "url"}}
	if len(got) != len(want) {
		t.Fatalf("sortRules(\"\") = %v (len=%d), want %v (len=%d)", got, len(got), want, len(want))
	}
	for i := range got {
		if got[i].Field != want[i].Field || got[i].Desc != want[i].Desc {
			t.Errorf("sortRules(\"\")[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSortRules_UnknownFallsBackToDateDesc(t *testing.T) {
	t.Parallel()

	got := sortRules("nonexistent_sort")
	want := []core.StorageSort{{Field: "date", Desc: true}, {Field: "url"}}
	if len(got) != len(want) {
		t.Fatalf("sortRules(\"nonexistent_sort\") = %v (len=%d), want %v (len=%d)", got, len(got), want, len(want))
	}
	for i := range got {
		if got[i].Field != want[i].Field || got[i].Desc != want[i].Desc {
			t.Errorf("sortRules(\"nonexistent_sort\")[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSortRules_DateAsc(t *testing.T) {
	t.Parallel()

	got := sortRules("date_asc")
	want := []core.StorageSort{{Field: "date"}, {Field: "url"}}
	if len(got) != len(want) {
		t.Fatalf("sortRules(\"date_asc\") = %v (len=%d), want %v (len=%d)", got, len(got), want, len(want))
	}
	for i := range got {
		if got[i].Field != want[i].Field || got[i].Desc != want[i].Desc {
			t.Errorf("sortRules(\"date_asc\")[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSortRules_NameAsc(t *testing.T) {
	t.Parallel()

	got := sortRules("name_asc")
	want := []core.StorageSort{{Field: "name"}, {Field: "url"}}
	if len(got) != len(want) {
		t.Fatalf("sortRules(\"name_asc\") = %v (len=%d), want %v (len=%d)", got, len(got), want, len(want))
	}
	for i := range got {
		if got[i].Field != want[i].Field || got[i].Desc != want[i].Desc {
			t.Errorf("sortRules(\"name_asc\")[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSortRules_DurationDesc(t *testing.T) {
	t.Parallel()

	got := sortRules("duration_desc")
	want := []core.StorageSort{{Field: "duration", Desc: true}, {Field: "url"}}
	if len(got) != len(want) {
		t.Fatalf("sortRules(\"duration_desc\") = %v (len=%d), want %v (len=%d)", got, len(got), want, len(want))
	}
	for i := range got {
		if got[i].Field != want[i].Field || got[i].Desc != want[i].Desc {
			t.Errorf("sortRules(\"duration_desc\")[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
