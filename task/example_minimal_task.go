// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

type ExampleMinimalTask struct {
	BaseTaskImpl
	objects []*model.DownloadObject
}

var _ core.Task = (*ExampleMinimalTask)(nil)

func NewExampleMinimalTask(cfg config.Task, store core.Storage) *ExampleMinimalTask {
	return &ExampleMinimalTask{
		BaseTaskImpl: BaseTaskImpl{
			id:      cfg.ID,
			saveDir: cfg.SaveDir,
			store:   store,
		},
		objects: make([]*model.DownloadObject, 0),
	}
}

func (t *ExampleMinimalTask) ID() string {
	return t.BaseTaskImpl.ID()
}

func (t *ExampleMinimalTask) Type() string {
	return "example_minimal"
}

func (t *ExampleMinimalTask) GetDownloadHeaders() map[string]string {
	return t.BaseTaskImpl.GetDownloadHeaders()
}

func (t *ExampleMinimalTask) GetDownloadObjects() ([]*model.DownloadObject, error) {
	return t.objects, nil
}

func (t *ExampleMinimalTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	return t.BaseTaskImpl.UpdateStatus(obj, status, err)
}

func (t *ExampleMinimalTask) Close() error {
	return t.BaseTaskImpl.Close()
}
