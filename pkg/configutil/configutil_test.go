// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package configutil

import "testing"

func TestGetInt64_ReturnsDefaultOnMissingKey(t *testing.T) {
	m := map[string]any{
		"present": int64(10),
	}
	if got, want := GetInt64(m, "missing", 42), int64(42); got != want {
		t.Fatalf("GetInt64 missing key = %d, want %d", got, want)
	}
}

func TestGetInt64_ReturnsDefaultOnNilValue(t *testing.T) {
	m := map[string]any{
		"k": nil,
	}
	if got, want := GetInt64(m, "k", 42), int64(42); got != want {
		t.Fatalf("GetInt64 nil value = %d, want %d", got, want)
	}
}

func TestGetInt64_StringConvertible(t *testing.T) {
	m := map[string]any{
		"k": "123",
	}
	if got, want := GetInt64(m, "k", 42), int64(123); got != want {
		t.Fatalf("GetInt64 convertible string = %d, want %d", got, want)
	}
}

func TestGetInt64_StringNotConvertible_ReturnsDefault(t *testing.T) {
	m := map[string]any{
		"k": "abc",
	}
	if got, want := GetInt64(m, "k", 42), int64(42); got != want {
		t.Fatalf("GetInt64 non-convertible string = %d, want %d", got, want)
	}
}

func TestGetBool_ReturnsDefaultOnMissingKey(t *testing.T) {
	m := map[string]any{
		"present": true,
	}
	if got, want := GetBool(m, "missing", true), true; got != want {
		t.Fatalf("GetBool missing key = %v, want %v", got, want)
	}
}

func TestGetBool_ReturnsDefaultOnNilValue(t *testing.T) {
	m := map[string]any{
		"k": nil,
	}
	if got, want := GetBool(m, "k", true), true; got != want {
		t.Fatalf("GetBool nil value = %v, want %v", got, want)
	}
}

func TestGetBool_StringConvertible(t *testing.T) {
	m := map[string]any{
		"k": "true",
	}
	if got, want := GetBool(m, "k", false), true; got != want {
		t.Fatalf("GetBool convertible string = %v, want %v", got, want)
	}
}

func TestGetString(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		key  string
		def  string
		want string
	}{
		{
			name: "key exists with string value",
			m:    map[string]any{"name": "hello"},
			key:  "name",
			def:  "default",
			want: "hello",
		},
		{
			name: "key missing returns default",
			m:    map[string]any{"name": "hello"},
			key:  "missing",
			def:  "fallback",
			want: "fallback",
		},
		{
			name: "key with non-string value returns default",
			m:    map[string]any{"count": 42},
			key:  "count",
			def:  "default",
			want: "default",
		},
		{
			name: "nil value returns default",
			m:    map[string]any{"name": nil},
			key:  "name",
			def:  "default",
			want: "default",
		},
		{
			name: "nil map returns default",
			m:    nil,
			key:  "anything",
			def:  "default",
			want: "default",
		},
		{
			name: "empty string value returned correctly",
			m:    map[string]any{"name": ""},
			key:  "name",
			def:  "default",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetString(tt.m, tt.key, tt.def)
			if got != tt.want {
				t.Errorf("GetString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetBool_StringNotConvertible_ReturnsDefault(t *testing.T) {
	m := map[string]any{
		"k": "nope",
	}
	if got, want := GetBool(m, "k", false), false; got != want {
		t.Fatalf("GetBool non-convertible string = %v, want %v", got, want)
	}
}
