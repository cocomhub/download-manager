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
	if cap, ok := t.(HeadersCap); ok {
		if cfg.Extra != nil {
			if v, ok := cfg.Extra["headers"]; ok {
				headers := map[string]string{}
				switch m := v.(type) {
				case map[string]string:
					for k, val := range m {
						if k != "" && val != "" {
							headers[k] = val
						}
					}
				case map[string]any:
					for k, val := range m {
						if k == "" || val == nil {
							continue
						}
						if s, ok := val.(string); ok && s != "" {
							headers[k] = s
						}
					}
				}
				if len(headers) > 0 {
					cap.SetHeaders(headers)
				}
			}
		}
	}
}
