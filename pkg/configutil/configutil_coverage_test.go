// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package configutil

import "testing"

func TestGetInt64_NilMap_ReturnsDefault(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		key  string
		def  int64
		want int64
	}{
		{
			name: "nil map returns default",
			m:    nil,
			key:  "anything",
			def:  99,
			want: 99,
		},
		{
			name: "normal int value (not string)",
			m:    map[string]any{"k": int(42)},
			key:  "k",
			def:  0,
			want: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetInt64(tt.m, tt.key, tt.def)
			if got != tt.want {
				t.Errorf("GetInt64() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetBool_NilMap_ReturnsDefault(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		key  string
		def  bool
		want bool
	}{
		{
			name: "nil map returns default",
			m:    nil,
			key:  "anything",
			def:  true,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetBool(tt.m, tt.key, tt.def)
			if got != tt.want {
				t.Errorf("GetBool() = %v, want %v", got, tt.want)
			}
		})
	}
}
