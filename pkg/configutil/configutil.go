// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package configutil provides helper functions for extracting typed values from
// map[string]any configuration maps (commonly used in config.Task.Extra).
package configutil

// GetString retrieves a string value from m by key, returning def if the key
// is missing, m is nil, or the value is not a string.
func GetString(m map[string]any, key, def string) string {
	if m == nil {
		return def
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}

// GetInt retrieves an int value from m by key. It handles both int and
// float64 (the default JSON number type) values, returning def if the key
// is missing, m is nil, or the value is not a number.
func GetInt(m map[string]any, key string, def int) int {
	if m == nil {
		return def
	}
	if v, ok := m[key].(int); ok {
		return v
	}
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return def
}

// GetBool retrieves a bool value from m by key, returning def if the key
// is missing, m is nil, or the value is not a bool.
func GetBool(m map[string]any, key string, def bool) bool {
	if m == nil {
		return def
	}
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}
