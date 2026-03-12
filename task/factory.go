package task

import (
	"fmt"

	"download-manager/config"
	"download-manager/core"
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
	return f(cfg, store)
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
