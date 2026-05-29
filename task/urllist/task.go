// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package urllist

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/dlcore"
	"github.com/cocomhub/download-manager/task"
)

const (
	TaskType = "url_list"
)

func init() {
	newFn := func(cfg *config.Task, opts task.Options) (core.Task, error) {
		return NewTask(cfg, opts)
	}
	task.Register(TaskType, newFn)
	task.Register("simple_url_list", newFn)
}

type Task struct {
	*task.BaseTask
}

// Ensure Task implements core.Task
var _ core.Task = (*Task)(nil)

func NewTask(cfg *config.Task, opts task.Options) (*Task, error) {
	bt, err := task.NewBaseTask(cfg, opts)
	if err != nil {
		return nil, err
	}
	t := &Task{
		BaseTask: bt,
	}

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

	usedNames := make(map[string]bool)
	for i, u := range urls {
		filename := filepath.Base(u)
		if filename == "." || filename == "/" {
			filename = fmt.Sprintf("file_%d.dat", i)
		}

		// Handle duplicates
		originalName := filename
		counter := 1
		for usedNames[filename] {
			ext := filepath.Ext(originalName)
			name := strings.TrimSuffix(originalName, ext)
			filename = fmt.Sprintf("%s_%d%s", name, counter, ext)
			counter++
		}
		usedNames[filename] = true

		_, err := url.Parse(u)
		if err != nil {
			return nil, fmt.Errorf("simple: url parse error: %s", err.Error())
		}

		obj := &model.DownloadObject{
			TaskID:   t.ID(),
			URL:      u,
			SavePath: filepath.Join(t.SaveDir(), filename),
			Status:   dlcore.StatusPending,
		}

		// Check storage for this object
		if storedObj := t.GetCachedObject(u); storedObj != nil {
			obj.Status = storedObj.Status
			obj.Metadata = storedObj.Metadata
			obj.Extra = storedObj.Extra
			t.ResetZombieState(obj)
		}

		err = t.UpdateStatus(obj, obj.Status, nil)
		if err != nil {
			return nil, err
		}
	}

	return t, nil
}

func (t *Task) Type() string {
	return TaskType
}

// Scrape implements core.ScrapeCap as a no-op — urllist's URL set is fixed at
// construction time and does not need page scraping.
func (t *Task) Scrape(ctx context.Context) error {
	return nil
}

func (t *Task) GetDownloadObjects() ([]*model.DownloadObject, error) {
	var pending []*model.DownloadObject
	for _, obj := range t.GetAllObjects(true) {
		if obj.Status != dlcore.StatusCompleted && obj.Status != dlcore.StatusCancelled {
			pending = append(pending, obj)
		}
	}
	return pending, nil
}
