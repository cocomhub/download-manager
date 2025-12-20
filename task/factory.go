package task

import (
	"fmt"

	"download-manager/config"
	"download-manager/core"
)

type Factory func(cfg config.TaskConfig, store core.Storage) (core.Task, error)

var factories = make(map[string]Factory)

func Register(typ string, f Factory) {
	factories[typ] = f
}

func NewTask(cfg config.TaskConfig, store core.Storage) (core.Task, error) {
	f, ok := factories[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unknown task type: %s", cfg.Type)
	}
	return f(cfg, store)
}

func init() {
	Register("simple_url_list", func(cfg config.TaskConfig, store core.Storage) (core.Task, error) {
		return NewSimpleTask(cfg.ID, cfg.URLs, cfg.SaveDir, store), nil
	})
}
