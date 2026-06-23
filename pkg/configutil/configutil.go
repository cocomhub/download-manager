// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package configutil provides helper functions for extracting typed values from
// map[string]any configuration maps (commonly used in config.Task.Extra).
package configutil

import (
	"github.com/spf13/cast"
)

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

func GetInt64(m map[string]any, key string, def int64) int64 {
	if m == nil {
		return def
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return def
	}
	v, err := cast.ToInt64E(raw)
	if err != nil {
		return def
	}
	return v
}

// GetBool retrieves a bool value from m by key, returning def if the key
// is missing, m is nil, or the value is not a bool.
func GetBool(m map[string]any, key string, def bool) bool {
	if m == nil {
		return def
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return def
	}
	v, err := cast.ToBoolE(raw)
	if err != nil {
		return def
	}
	return v
}
