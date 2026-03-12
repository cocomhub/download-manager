// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"fmt"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
)

type Factory func(cfg config.Task, store core.Storage) (core.Task, error)

var factories = make(map[string]Factory)

func Register(typ string, f Factory) {
	factories[typ] = f
}

func NewTask(cfg config.Task, store core.Storage) (core.Task, error) {
	f, ok := factories[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unknown task type: %s", cfg.Type)
	}
	t, err := f(cfg, store)
	if err != nil {
		return nil, err
	}
	wireByCapabilities(cfg, t)
	return t, nil
}

func init() {
	Register("simple_url_list", func(cfg config.Task, store core.Storage) (core.Task, error) {
		var urls []string
		if cfg.Extra != nil {
			if v, ok := cfg.Extra["urls"]; ok {
				switch vv := v.(type) {
				case []string:
					urls = vv
				case []any:
					for _, it := range vv {
						if s, ok := it.(string); ok && s != "" {
							urls = append(urls, s)
						}
					}
				}
			}
		}
		return NewSimpleTask(cfg.ID, urls, cfg.SaveDir, store), nil
	})
	Register("tktube", func(cfg config.Task, store core.Storage) (core.Task, error) {
		return NewTktubeTask(cfg, store)
	})
	Register("vikacg", func(cfg config.Task, store core.Storage) (core.Task, error) {
		return NewVikacgTask(cfg, store)
	})
	Register("hanime", func(cfg config.Task, store core.Storage) (core.Task, error) {
		return NewHanimeTask(cfg, store)
	})
}

func wireByCapabilities(cfg config.Task, t core.Task) {
	if cap, ok := t.(PathStrategyCap); ok {
		mode := "first_fixed"
		if cfg.Extra != nil {
			if v, ok := cfg.Extra["path_strategy"]; ok {
				if s, ok2 := v.(string); ok2 && s != "" {
					mode = s
				}
			}
		}
		cap.SetPathStrategy(NewPathStrategy(mode, cfg.SaveDir))
	}
	if cap, ok := t.(RefreshingCap); ok {
		sec := 3600
		if cfg.Extra != nil {
			if v, ok := cfg.Extra["refresh_interval"]; ok {
				switch vv := v.(type) {
				case int:
					sec = vv
				case float64:
					sec = int(vv)
				}
			}
		}
		r := NewCommonRefresher(sec)
		cap.SetRefresher(r)
	}
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
