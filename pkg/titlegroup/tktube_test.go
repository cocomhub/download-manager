// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package titlegroup

import "testing"

func TestTKTGroupNameFromTitle(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"CLUB-100", "CLUB-100"},
		{"CLUB-100C", "CLUB-100"},
		{"銆愰珮鐢昏川銆慍LUB-100", "CLUB-100"},
		{"銆愰珮鐢昏川銆慍LUB-100C", "CLUB-100"},
		{"SSIS-123", "SSIS-123"},
		{"ABP-456C", "ABP-456"},
		{"闅忔満鏍囬", ""},
	}
	for _, c := range cases {
		got := TKTGroupNameFromTitle(c.in)
		if got != c.want {
			t.Fatalf("input=%q: want %q, got %q", c.in, c.want, got)
		}
	}
}

func TestTKTContentGroupKey(t *testing.T) {
	cases := []struct {
		name  string
		title string
		url   string
		want  string
	}{
		{name: "鍚堟硶缁勫悕", title: "銆愰珮鐢昏川銆慍LUB-100C", url: "https://example.com/a", want: "CLUB-100"},
		{name: "鏈煡鏍囬", title: "  闅忔満鏍囬  ", url: "https://example.com/b", want: "unknown+闅忔満鏍囬"},
		{name: "绌烘爣棰?, title: "   ", url: "https://example.com/c", want: "unknown+https://example.com/c"},
		{name: "鍏ㄧ┖浠嶉潪绌?, title: "", url: "", want: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TKTContentGroupKey(tc.title, tc.url)
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
			if got == "" {
				t.Fatal("content group key should never be empty")
			}
		})
	}
}
