// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"fmt"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
)

type Factory func(cfg *config.Task, opts Options) (core.Task, error)

var factories = make(map[string]Factory)

func Register(typ string, f Factory) {
	factories[typ] = f
}

func NewTask(cfg *config.Task, opts ...Option) (core.Task, error) {
	f, ok := factories[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unknown task type: %s", cfg.Type)
	}

	o := Options{}
	for _, opt := range opts {
		opt(&o)
	}

	t, err := f(cfg, o)
	if err != nil {
		return nil, err
	}
	wireByCapabilities(cfg, t)
	return t, nil
}

func wireByCapabilities(cfg *config.Task, t core.Task) {
	cap, ok := t.(HeadersSetter)
	if !ok {
		return
	}

	headers := extractHeaders(cfg)
	if len(headers) > 0 {
		cap.SetHeaders(headers)
	}
}

// extractHeaders extracts and normalizes the "headers" map from cfg.Extra.
// It handles both map[string]string and map[string]any types, filtering out
// empty keys, empty values, and nil values.
func extractHeaders(cfg *config.Task) map[string]string {
	if cfg.Extra == nil {
		return nil
	}

	v, ok := cfg.Extra["headers"]
	if !ok {
		return nil
	}

	switch m := v.(type) {
	case map[string]string:
		return filterStringMap(m)
	case map[string]any:
		return convertAnyMap(m)
	}
	return nil
}

// filterStringMap copies a map[string]string, keeping only non-empty entries.
func filterStringMap(m map[string]string) map[string]string {
	headers := make(map[string]string, len(m))
	for k, val := range m {
		if k != "" && val != "" {
			headers[k] = val
		}
	}
	return headers
}

// convertAnyMap converts a map[string]any to map[string]string,
// keeping only entries where the value is a non-empty string.
func convertAnyMap(m map[string]any) map[string]string {
	headers := make(map[string]string, len(m))
	for k, val := range m {
		if k == "" || val == nil {
			continue
		}
		if s, ok := val.(string); ok && s != "" {
			headers[k] = s
		}
	}
	return headers
}
