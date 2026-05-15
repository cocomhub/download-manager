// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

type ExampleMinimalTask struct {
	BaseTask
}

var _ core.Task = (*ExampleMinimalTask)(nil)

func NewExampleMinimalTask(cfg config.Task, store core.Storage) *ExampleMinimalTask {
	t := &ExampleMinimalTask{
		BaseTask: NewBaseTask(cfg.ID, cfg.SaveDir, store),
	}
	return t
}

func (t *ExampleMinimalTask) Type() string {
	return "example_minimal"
}

func (t *ExampleMinimalTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	return t.GetAllObjects(), nil
}
