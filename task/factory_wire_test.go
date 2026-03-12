// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

type fakeTask struct {
	id        string
	ps        core.PathStrategy
	refresher *CommonRefresher
	headers   map[string]string
}

var _ core.Task = (*fakeTask)(nil)
var _ PathStrategyCap = (*fakeTask)(nil)
var _ RefreshingCap = (*fakeTask)(nil)
var _ HeadersCap = (*fakeTask)(nil)

func (f *fakeTask) ID() string                                           { return f.id }
func (f *fakeTask) GetDownloadHeaders() map[string]string                { return f.headers }
func (f *fakeTask) GetDownloadObjects() ([]*model.DownloadObject, error) { return nil, nil }
func (f *fakeTask) UpdateStatus(obj *model.DownloadObject, status string, err error) error {
	return nil
}
func (f *fakeTask) Type() string { return "fake_for_test" }
func (f *fakeTask) Close() error { return nil }

func (f *fakeTask) SetPathStrategy(ps core.PathStrategy) { f.ps = ps }
func (f *fakeTask) SetRefresher(r *CommonRefresher)      { f.refresher = r }
func (f *fakeTask) SetHeaders(h map[string]string)       { f.headers = h }

func TestFactoryWireByCapabilities(t *testing.T) {
	Register("fake_for_test", func(cfg config.Task, store core.Storage) (core.Task, error) {
		return &fakeTask{id: cfg.ID}, nil
	})

	cfg := config.Task{
		ID:      "f1",
		Type:    "fake_for_test",
		SaveDir: "/tmp/save",
		Extra: map[string]any{
			"path_strategy":    "first_fixed",
			"refresh_interval": 123,
			"headers":          map[string]any{"X-Token": "abc"},
		},
	}

	tk, err := NewTask(cfg, nil)
	if err != nil {
		t.Fatalf("NewTask error: %v", err)
	}
	ft := tk.(*fakeTask)

	if ft.ps == nil {
		t.Fatalf("path strategy not injected")
	}
	if ft.refresher == nil {
		t.Fatalf("refresher not injected")
	}
	if ft.refresher.interval != 123*time.Second {
		t.Fatalf("refresher.interval = %v, want %v", ft.refresher.interval, 123*time.Second)
	}
	if ft.headers == nil {
		t.Fatalf("headers not injected")
	}
	if ft.headers["X-Token"] != "abc" {
		t.Fatalf("headers[X-Token] = %q, want %q", ft.headers["X-Token"], "abc")
	}
}
